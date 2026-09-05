package ru.tinyops.turboist.nativeapp.search

import android.util.Log
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.database.search.FtsQuery

/** Everything the search screen renders. */
data class SearchUiState(
    val typed: String = "",
    val filters: SearchFilters = SearchFilters(),
    val searching: Boolean = false,
    val results: SearchResults = SearchResults(),
    /**
     * True once a query has been answered. Tells "found nothing" apart from
     * "has not been asked yet", which are different screens entirely.
     */
    val answered: Boolean = false,
    val recent: List<String> = emptyList(),
) {
    /** True while what is typed is too short to be worth asking about. */
    val resting: Boolean
        get() = FtsQuery.match(typed) == null
}

/**
 * The behaviour of the search screen.
 *
 * A plain object driven by a scope rather than a view model, so all of it can be
 * exercised without Compose and without the platform.
 *
 * Two things about it are worth stating, because both are decisions rather than
 * mechanics:
 *
 * - **Typing is debounced; everything else is not.** A keystroke is not a
 *   request to search, it is part of one, so a query waits a moment for the next
 *   letter. Changing a filter, tapping a past search or pressing the keyboard's
 *   search key *are* the request, and waiting after them would only feel slow.
 *   Each new run cancels the one in flight, so the answer on screen is always
 *   the answer to what is on screen.
 * - **Only a deliberate search is remembered.** Recording every keystroke would
 *   fill the recent list with the prefixes of one query. What gets remembered is
 *   what the user submitted or re-ran.
 *
 * Nothing here waits on a network, because nothing it does involves one: the
 * index is on the device. That is why there is no retry, no error banner and no
 * offline state — a search either has an answer or has not been asked.
 */
class SearchPresenter(
    private val scope: CoroutineScope,
    private val repository: SearchRepository,
    private val recent: RecentSearches,
    private val debounceMillis: Long = TYPING_DEBOUNCE_MILLIS,
) {
    private val internal = MutableStateFlow(SearchUiState())

    /** What the screen renders. */
    val state: StateFlow<SearchUiState> = internal.asStateFlow()

    private var running: Job? = null

    init {
        scope.launch {
            recent.observe().collect { queries -> internal.update { it.copy(recent = queries) } }
        }
    }

    /** The user typed. */
    fun type(typed: String) {
        internal.update { it.copy(typed = typed) }
        run(delayMillis = debounceMillis)
    }

    /** The user chose which kind of thing to look at, or chose to see all four. */
    fun narrowTo(kind: SearchKind?) {
        internal.update { it.copy(filters = it.filters.copy(kind = kind)) }
        run(delayMillis = 0)
    }

    /** The user turned finished work out of the results, or back into them. */
    fun toggleOpenTasksOnly() {
        internal.update { it.copy(filters = it.filters.copy(openTasksOnly = !it.filters.openTasksOnly)) }
        run(delayMillis = 0)
    }

    /** The user asked for the search now — the keyboard's search key, or the button. */
    fun submit() {
        run(delayMillis = 0, remember = true)
    }

    /** The user tapped one of their past searches. */
    fun rerun(query: String) {
        internal.update { it.copy(typed = query) }
        run(delayMillis = 0, remember = true)
    }

    /** The user emptied the field. */
    fun clearQuery() {
        type("")
    }

    /** The user asked to forget what they had searched for before. */
    fun forgetRecent() {
        scope.launch { recent.clear() }
    }

    private fun run(
        delayMillis: Long,
        remember: Boolean = false,
    ) {
        running?.cancel()
        val asked = internal.value
        if (asked.resting) {
            // Not an empty result — an unasked question. The screen goes back to
            // its resting state rather than reporting that nothing matched.
            internal.update { it.copy(searching = false, answered = false, results = SearchResults()) }
            return
        }
        running =
            scope.launch {
                if (delayMillis > 0) delay(delayMillis)
                internal.update { it.copy(searching = true) }
                val found =
                    try {
                        repository.search(asked.typed, asked.filters)
                    } catch (failure: RuntimeException) {
                        // The index lives on this device, so a failure here is a
                        // defect rather than a condition the user can act on:
                        // report it where a developer will see it and leave the
                        // screen saying that nothing was found.
                        Log.w(LOG_TAG, "Searching the local index failed", failure)
                        SearchResults()
                    }
                internal.update { it.copy(searching = false, answered = true, results = found) }
                if (remember) recent.remember(asked.typed)
            }
    }

    private companion object {
        const val LOG_TAG = "TurboistSearch"

        /**
         * How long a keystroke waits for the next one.
         *
         * The same pause the web client uses, so the two feel alike. Long enough
         * that a word typed at speed is one search rather than six, short enough
         * that it never reads as lag.
         */
        const val TYPING_DEBOUNCE_MILLIS = 300L
    }
}
