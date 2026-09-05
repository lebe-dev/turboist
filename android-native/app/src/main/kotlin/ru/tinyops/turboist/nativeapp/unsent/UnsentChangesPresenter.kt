package ru.tinyops.turboist.nativeapp.unsent

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** The two things the unsent-changes screen can ask for. */
interface UnsentChangesActions {
    /**
     * Closes one refusal.
     *
     * Nothing is undone by it. The change never happened on the server, and the
     * row it was made against has already been corrected by a catch-up, so what
     * is being discarded is the note about the refusal rather than the work.
     */
    suspend fun discard(id: String): Boolean

    /** Closes every refusal at once, for the user who wants the list gone rather than read. */
    suspend fun discardAll(): Int
}

/** Everything the unsent-changes screen renders. */
data class UnsentChangesUiState(
    val loading: Boolean = true,
    /** What the server refused. Only a person can clear these. */
    val setAside: List<UnsentChange> = emptyList(),
    /** What is still queued, shown so the queue is never a black box. */
    val waiting: List<UnsentChange> = emptyList(),
    /** True while the "discard everything" confirmation is up. */
    val confirmingDiscardAll: Boolean = false,
) {
    /** True once the device is known to be holding nothing back, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && setAside.isEmpty() && waiting.isEmpty()
}

/**
 * The behaviour of the unsent-changes screen.
 *
 * Two piles, and they are not the same thing. What is *waiting* needs nothing
 * from anyone: it goes out when the server can be reached, and it is listed only
 * so the queue is visible rather than a rumour — nothing on it can be discarded
 * from here, because throwing away a change that is still going to land would be
 * the data loss this screen exists to prevent.
 *
 * What is *set aside* is the pile that needs a person. The server answered no,
 * so it will never go out, and the only thing left to decide is whether the user
 * wants to read the note or be rid of it.
 *
 * A plain object driven by a scope, like every other presenter here, so all of
 * it runs without Compose and without the platform.
 */
class UnsentChangesPresenter(
    private val scope: CoroutineScope,
    setAside: Flow<List<UnsentChange>>,
    waiting: Flow<List<UnsentChange>>,
    private val actions: UnsentChangesActions,
) {
    private val confirming = MutableStateFlow(false)

    /** What the screen renders. */
    val state: StateFlow<UnsentChangesUiState> =
        combine(setAside, waiting, confirming) { refused, queued, askingToDiscardAll ->
            UnsentChangesUiState(
                loading = false,
                setAside = refused,
                waiting = queued,
                // The confirmation follows the pile it is about: a queue drained
                // on another thread while the dialog was up leaves a dialog
                // asking about nothing, so it closes itself instead.
                confirmingDiscardAll = askingToDiscardAll && refused.isNotEmpty(),
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), UnsentChangesUiState())

    /** Closes one refusal. */
    fun discard(change: UnsentChange) {
        scope.launch { actions.discard(change.id) }
    }

    /** Asks before clearing the whole pile: it is the one action here that cannot be taken back. */
    fun askToDiscardAll() {
        confirming.value = true
    }

    fun cancelDiscardAll() {
        confirming.value = false
    }

    fun discardAll() {
        scope.launch {
            try {
                actions.discardAll()
            } finally {
                confirming.value = false
            }
        }
    }

    private companion object {
        /** Long enough to survive a screen rotation without tearing the queries down. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
