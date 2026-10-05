package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import java.time.LocalDate
import java.time.ZoneId

/** Everything the completion history screen renders. */
data class CompletedUiState(
    val loading: Boolean = true,
    val sections: List<CompletedDaySection> = emptyList(),
    val refreshing: Boolean = false,
    val older: OlderHistory = OlderHistory.LOADING,
) {
    /** True once the history is known to hold nothing, as opposed to not being read yet. */
    val isEmpty: Boolean
        get() = !loading && sections.isEmpty()
}

/**
 * The finished work the device itself holds.
 *
 * A seam rather than a repository call so the screen's behaviour can be exercised
 * without a database: what is under test here is what happens at the edge of the
 * device's copy, not how the copy is queried.
 */
interface CompletedHistorySource {
    /** The most recent [limit] completions the replica holds, most recent first. */
    fun observe(limit: Int): Flow<List<Task>>

    /** How many completions the replica holds altogether, however few are being drawn. */
    fun observeCount(): Flow<Int>

    /** The title of each project, by the device's own id for it. */
    fun observeProjectTitles(): Flow<Map<Long, String>>

    /**
     * The instant the device's copy of the history begins at.
     *
     * It is the boundary the two sources are split on, and it is read afresh each
     * time rather than captured: the window is measured back from today, so it
     * moves when the day does.
     */
    fun windowStart(): Long
}

/** The one write the completion history can make. */
interface CompletedHistoryActions {
    /**
     * Puts a finished task back into the open work.
     *
     * It takes the whole task rather than an id because a line of this screen may
     * be one the device has no copy of: such a task is named the way the server
     * names it, and turning that into something the queue can carry is the write
     * path's business, not the screen's.
     */
    suspend fun uncomplete(task: Task)
}

/**
 * The completion history: what the device holds, and the way past the end of it.
 *
 * The screen has two sources and one list. The device's own copy answers
 * instantly and works with no signal, and it covers a bounded stretch of time;
 * everything before that stretch is on the server and is fetched only when
 * somebody scrolls that far. The seam between them is the whole of this class,
 * and the rule it keeps is that the boundary is never dressed up: past the edge
 * of the replica the screen either fetches, or says plainly that fetching needs a
 * connection. It never shows a spinner for something that is not on its way.
 *
 * Fetched pages are held here and nowhere else. Writing them into the replica
 * would put rows there that the next tidy-up removes again, so scrolling back
 * through a year of history would rewrite the database on every pass.
 */
class CompletedHistoryPresenter(
    private val scope: CoroutineScope,
    private val history: CompletedHistorySource,
    private val older: OlderCompletions,
    private val actions: CompletedHistoryActions,
    private val sync: SyncScheduler,
    private val zone: ZoneId,
    today: Flow<LocalDate>,
) {
    private val shown = MutableStateFlow(PAGE)
    private val fetched = MutableStateFlow<List<Task>>(emptyList())
    private val refreshing = MutableStateFlow(false)

    /** What the fetching half of the screen is doing, or `null` while nothing has asked it to. */
    private val fetching = MutableStateFlow<OlderHistory?>(null)

    private val outgoing = MutableSharedFlow<TaskListMessage>(extraBufferCapacity = 4)

    /** Where the next page from the server starts, once the first one has settled it. */
    private var nextOffset: Int? = null

    private var inFlight = false

    /** What the screen renders. */
    @OptIn(ExperimentalCoroutinesApi::class)
    val state: StateFlow<CompletedUiState> =
        combine(
            shown.flatMapLatest { limit -> history.observe(limit) },
            history.observeCount(),
            fetched,
            history.observeProjectTitles(),
            combine(today, refreshing, fetching) { day, isRefreshing, fetchState ->
                Triple(day, isRefreshing, fetchState)
            },
        ) { replicated, held, pages, titles, moment ->
            val (day, isRefreshing, fetchState) = moment
            CompletedUiState(
                loading = false,
                sections = completedDaySections(completedRows(replicated, pages, titles), zone, day),
                refreshing = isRefreshing,
                older =
                    when {
                        replicated.size < held -> OlderHistory.IN_REPLICA
                        else -> fetchState ?: OlderHistory.ON_SERVER
                    },
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), CompletedUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<TaskListMessage> = outgoing.asSharedFlow()

    /**
     * Shows more history: the next slice of the device's own copy while there is
     * one, and the server's after that.
     *
     * Both are the same gesture on purpose. The user is scrolling backwards
     * through time and should not have to know which side of the device's window
     * they are on — the only moment that distinction is allowed to show is when
     * there is no connection and the server's half cannot be had.
     */
    fun showMore() {
        when (state.value.older) {
            OlderHistory.IN_REPLICA -> shown.value += PAGE
            OlderHistory.ON_SERVER, OlderHistory.OFFLINE, OlderHistory.FAILED -> fetchOlder()
            OlderHistory.LOADING, OlderHistory.EXHAUSTED -> Unit
        }
    }

    /**
     * Asks the sync engine to catch up, and forgets the pages fetched for this
     * screen.
     *
     * They are dropped rather than kept because they are a copy taken at a moment
     * that the gesture has just declared stale, and because the replica's window
     * has to be redrawn from the boundary outwards for the two halves to still
     * meet.
     */
    fun refresh() {
        scope.launch {
            refreshing.value = true
            try {
                sync.requestSyncNow()
            } finally {
                refreshing.value = false
                fetched.value = emptyList()
                nextOffset = null
                fetching.value = null
            }
        }
    }

    /**
     * Puts a finished task back into the open work.
     *
     * A line the device holds is written straight to the replica and leaves the
     * history at once. A fetched line has no copy here to change, so it is dropped
     * from this screen's pages and comes back as an open task with the catch-up
     * that follows the queued write.
     */
    fun uncomplete(row: CompletedRow) {
        scope.launch {
            try {
                actions.uncomplete(row.task)
            } catch (refusal: WriteRefused) {
                outgoing.emit(TaskListMessage.FAILED)
                return@launch
            } catch (failure: RuntimeException) {
                outgoing.emit(TaskListMessage.FAILED)
                return@launch
            }
            if (row.source == CompletedSource.SERVER) {
                fetched.value = fetched.value.filterNot { it.serverId == row.task.serverId }
            }
        }
    }

    /**
     * Reads pages from the server until one of them carries history the device
     * does not have.
     *
     * The first request starts where the device's own copy ends, which is what
     * keeps the common case to a single round trip. That starting point is an
     * estimate — it is the count the device holds, and the server's count for the
     * same stretch can differ by whatever changed in between — so a page that
     * turns out to hold nothing new is stepped over rather than treated as the end
     * of the history. Only a limited number of them: a boundary that has drifted
     * by a page or two is worth stepping over, a hunt through the entire history
     * is not, and stopping leaves the screen offering to go on rather than
     * claiming there is nothing there.
     */
    private fun fetchOlder() {
        if (inFlight) return
        inFlight = true
        fetching.value = OlderHistory.LOADING
        scope.launch {
            try {
                var offset = nextOffset ?: history.observeCount().first()
                var skipped = 0
                while (true) {
                    when (val page = older.page(offset, PAGE)) {
                        is OlderCompletionsResult.Offline -> {
                            fetching.value = OlderHistory.OFFLINE
                            return@launch
                        }

                        is OlderCompletionsResult.Failed -> {
                            fetching.value = OlderHistory.FAILED
                            return@launch
                        }

                        is OlderCompletionsResult.Page -> {
                            offset = page.nextOffset
                            nextOffset = offset
                            val boundary = history.windowStart()
                            val beyond = page.tasks.filter { (it.completedAt ?: Long.MAX_VALUE) < boundary }
                            if (beyond.isNotEmpty()) {
                                fetched.value = merge(fetched.value, beyond)
                                fetching.value =
                                    if (page.hasMore) OlderHistory.ON_SERVER else OlderHistory.EXHAUSTED
                                return@launch
                            }
                            if (!page.hasMore) {
                                fetching.value = OlderHistory.EXHAUSTED
                                return@launch
                            }
                            if (++skipped >= MAX_PAGES_STEPPED_OVER) {
                                fetching.value = OlderHistory.ON_SERVER
                                return@launch
                            }
                        }
                    }
                }
            } finally {
                inFlight = false
            }
        }
    }

    /** Adds a page, keeping out anything already held — a repeated request must not double a row. */
    private fun merge(
        held: List<Task>,
        page: List<Task>,
    ): List<Task> {
        val known = held.mapNotNull { it.serverId }.toSet()
        return held + page.filter { it.serverId == null || it.serverId !in known }
    }

    private companion object {
        /** How many more lines one "show more" adds, on either side of the boundary. */
        const val PAGE = 50

        /**
         * How many pages of history the device already holds may be read past
         * while looking for the boundary.
         */
        const val MAX_PAGES_STEPPED_OVER = 3

        /** Matches the list screens, so a rotation does not tear the queries down. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
