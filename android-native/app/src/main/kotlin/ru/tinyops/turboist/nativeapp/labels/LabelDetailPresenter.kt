package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Label

/** Everything the header of a label's own screen renders. */
data class LabelDetailUiState(
    val loading: Boolean = true,
    val label: Label? = null,
    /** How many tasks carry the label, which the delete warning names. */
    val taggedTasks: Int = 0,
) {
    /**
     * True once the label is known to be gone, as opposed to not being read yet.
     *
     * The screen is a standing query, so this becomes true the moment the label
     * is deleted — here or on another device — and the screen says so instead of
     * showing the last thing it happened to hold.
     */
    val missing: Boolean get() = !loading && label == null
}

/**
 * The behaviour of one label's own screen.
 *
 * The tasks carrying the label are not here: they are a task list like any other
 * and are read, drawn and acted on by the shared list machinery, so ticking a
 * row off means exactly what it means everywhere else. What is left is the label
 * itself — renaming it, recolouring it, marking it, and taking it away.
 */
class LabelDetailPresenter(
    private val scope: CoroutineScope,
    label: Flow<Label?>,
    taggedTasks: Flow<Int>,
    private val actions: LabelActions,
) {
    private val outgoing = MutableSharedFlow<LabelMessage>(extraBufferCapacity = 1)

    val state: StateFlow<LabelDetailUiState> =
        combine(label, taggedTasks) { current, tagged ->
            LabelDetailUiState(loading = false, label = current, taggedTasks = tagged)
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), LabelDetailUiState())

    val messages: SharedFlow<LabelMessage> = outgoing.asSharedFlow()

    /**
     * Renames and recolours the label in one change.
     *
     * One change rather than two, because the editor asks both questions at
     * once: two writes would queue two requests and let the phone come back from
     * a tunnel having renamed the label but not recoloured it.
     *
     * A blank name is not a name, so nothing is written.
     */
    fun save(
        name: String,
        color: String,
    ) {
        val named = name.trim()
        if (named.isEmpty()) return
        edit(LabelEdit(name = named, color = color))
    }

    fun toggleFavourite() {
        val current = state.value.label ?: return
        edit(LabelEdit(isFavourite = !current.isFavourite))
    }

    fun togglePrivate() {
        val current = state.value.label ?: return
        edit(LabelEdit(isPrivate = !current.isPrivate))
    }

    /**
     * Takes the label away, and with it every tagging of it.
     *
     * The tasks are untouched — a tagging is an edge, not the work — which is
     * what the warning in front of this says, with the number of tasks about to
     * lose it.
     */
    fun delete() {
        val current = state.value.label ?: return
        scope.launch { outgoing.reporting { actions.deleteLabel(current.localId) } }
    }

    private fun edit(change: LabelEdit) {
        val current = state.value.label ?: return
        scope.launch { outgoing.reporting { actions.editLabel(current.localId, change) } }
    }

    private companion object {
        /** Matches the list presenters' grace period, so a screen stops observing all at once. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
