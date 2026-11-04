package ru.tinyops.turboist.nativeapp.switcher

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddActions
import java.time.LocalDate
import java.time.ZoneId

/** Where a task typed into the quick switcher is filed. */
enum class SwitcherTarget {
    /** The inbox, undated. */
    INBOX,

    /** Due today, in the project opened most recently — or the inbox when there is none. */
    TODAY,
}

/** What the quick switcher renders. */
data class QuickSwitcherUiState(
    val visible: Boolean = false,
    val typed: String = "",
    val rows: List<SwitcherRow> = emptyList(),
    /** The project "for today" files into; `null` means the inbox. */
    val todayProject: Project? = null,
    val creating: Boolean = false,
) {
    /** The title a created task would get: what was typed, with its spacing tidied. */
    val title: String
        get() = typed.trim().replace(WHITESPACE, " ")
}

private val WHITESPACE = Regex("\\s+")

/** A result the screen announces once, after the panel has closed. */
sealed interface QuickSwitcherMessage {
    data object AddedToInbox : QuickSwitcherMessage

    data class AddedForToday(
        val projectTitle: String?,
    ) : QuickSwitcherMessage

    data object Failed : QuickSwitcherMessage
}

/**
 * The behaviour of the quick switcher: find a task or project by what is typed,
 * recently opened ones first, or file what is typed as a new task.
 *
 * A plain object on a scope rather than a view model, so all of it can be checked
 * without Compose. Every answer comes from the replica and every write goes
 * through the outbox, so none of it needs a connection.
 */
class QuickSwitcherPresenter(
    private val scope: CoroutineScope,
    private val repository: QuickSwitcherRepository,
    private val recentItems: RecentItems,
    private val actions: QuickAddActions,
    private val zone: ZoneId,
    private val today: () -> LocalDate,
) {
    private val internal = MutableStateFlow(QuickSwitcherUiState())
    private val outbound = Channel<QuickSwitcherMessage>(Channel.BUFFERED)

    /** What the panel renders. */
    val state: StateFlow<QuickSwitcherUiState> = internal.asStateFlow()

    /** One-off announcements: what was added, or that adding failed. */
    val messages: Flow<QuickSwitcherMessage> = outbound.receiveAsFlow()

    private var recents: List<RecentRef> = emptyList()
    private var running: Job? = null

    init {
        scope.launch {
            recentItems.observe().collect { refs ->
                recents = refs
                if (internal.value.visible) refresh(delayMillis = 0)
            }
        }
    }

    /** The panel was asked for. It always opens empty, on the recents. */
    fun open() {
        internal.update { QuickSwitcherUiState(visible = true) }
        refresh(delayMillis = 0)
    }

    /** The panel was put away without choosing anything. */
    fun dismiss() {
        running?.cancel()
        internal.update { QuickSwitcherUiState() }
    }

    /** The user typed. */
    fun type(typed: String) {
        internal.update { it.copy(typed = typed) }
        refresh(delayMillis = if (typed.isBlank()) 0 else TYPING_DEBOUNCE_MILLIS)
    }

    /** A task or project was opened — from the panel or anywhere else in the app. */
    fun visit(ref: RecentRef) {
        scope.launch { recentItems.visit(ref) }
    }

    /** The user chose one of the two "add" rows. */
    fun create(target: SwitcherTarget) {
        val asked = internal.value
        val title = asked.title
        if (title.isEmpty() || asked.creating) return
        internal.update { it.copy(creating = true) }
        scope.launch {
            val message = write(target, title, asked.todayProject)
            if (message is QuickSwitcherMessage.Failed) {
                internal.update { it.copy(creating = false) }
            } else {
                dismiss()
            }
            outbound.send(message)
        }
    }

    // Any failure of the local write — a refusal, a storage error — must leave
    // the panel open with what was typed, so it is caught whole.
    @Suppress("TooGenericExceptionCaught")
    private suspend fun write(
        target: SwitcherTarget,
        title: String,
        project: Project?,
    ): QuickSwitcherMessage =
        try {
            when (target) {
                SwitcherTarget.INBOX -> {
                    actions.create(TaskDestination.Inbox, NewTask(title = title))
                    QuickSwitcherMessage.AddedToInbox
                }
                SwitcherTarget.TODAY -> {
                    val dueAt = today().atStartOfDay(zone).toInstant().toEpochMilli()
                    val destination = project?.let { TaskDestination.InProject(it.localId) } ?: TaskDestination.Inbox
                    actions.create(destination, NewTask(title = title, dueAt = dueAt, dueHasTime = false))
                    QuickSwitcherMessage.AddedForToday(project?.title)
                }
            }
        } catch (refusal: WriteRefused) {
            Log.i(LOG_TAG, "The quick switcher could not add a task: ${refusal.code}")
            QuickSwitcherMessage.Failed
        } catch (failure: RuntimeException) {
            Log.w(LOG_TAG, "Adding a task from the quick switcher failed", failure)
            QuickSwitcherMessage.Failed
        }

    // The index lives on this device, so a failed read is a defect rather than
    // something the user can act on: it is reported and the panel shows less.
    @Suppress("TooGenericExceptionCaught")
    private suspend fun <T> readOrElse(
        what: String,
        fallback: T,
        read: suspend () -> T,
    ): T =
        try {
            read()
        } catch (failure: RuntimeException) {
            Log.w(LOG_TAG, "Reading $what for the quick switcher failed", failure)
            fallback
        }

    private fun refresh(delayMillis: Long) {
        running?.cancel()
        val typed = internal.value.typed
        val refs = recents
        running =
            scope.launch {
                if (delayMillis > 0) delay(delayMillis)
                val resolved = readOrElse("the recently opened rows", emptyList()) { repository.resolve(refs) }
                val matches =
                    if (typed.isBlank()) {
                        SwitcherMatches()
                    } else {
                        readOrElse("the local index", SwitcherMatches()) { repository.search(typed) }
                    }
                val todayProject =
                    resolved
                        .filterIsInstance<SwitcherRow.ProjectRow>()
                        .firstOrNull { it.project.status == ProjectStatus.OPEN }
                        ?.project
                internal.update {
                    if (it.typed != typed) {
                        it
                    } else {
                        it.copy(
                            rows = switcherRows(typed, resolved, matches.tasks, matches.projects),
                            todayProject = todayProject,
                        )
                    }
                }
            }
    }

    private companion object {
        const val LOG_TAG = "TurboistSwitcher"

        /** Shorter than the search screen's pause: the panel is for a few letters and a tap. */
        const val TYPING_DEBOUNCE_MILLIS = 150L
    }
}
