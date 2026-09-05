package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.WriteRefused
import java.time.LocalDate
import java.time.ZoneId

/**
 * The one write capture makes.
 *
 * A narrow port onto the write path, so the sheet depends on the single thing it
 * does rather than on the whole catalogue of task mutations — and so a check
 * about the sheet needs no database.
 */
interface QuickAddActions {
    suspend fun create(
        destination: TaskDestination,
        task: NewTask,
    )
}

/** Everything the capture sheet renders. */
data class QuickAddUiState(
    val visible: Boolean = false,
    val draft: QuickAddDraft = QuickAddDraft(),
    val submitting: Boolean = false,
    /** True while the sheet is showing the list of places instead of the fields. */
    val pickingProject: Boolean = false,
    val projectQuery: String = "",
    /** The places this device filed work into lately, leading the picker. */
    val recentProjects: List<Project> = emptyList(),
    /** Everything else the picker offers, with the recent ones lifted out. */
    val otherProjects: List<Project> = emptyList(),
    /** Offered for the typed title, never applied. */
    val suggestedProjects: List<Project> = emptyList(),
    /** Will be attached on save unless taken off. */
    val autoLabels: List<String> = emptyList(),
    val knownLabels: List<Label> = emptyList(),
    /** The chosen project's title, or `null` while the draft is headed for the inbox. */
    val projectTitle: String? = null,
)

/**
 * Something the capture sheet has to say back after a save.
 *
 * Carried as a value rather than as words, so the presenter stays free of
 * resources and of a language; the screen turns it into a sentence.
 */
enum class QuickAddMessage {
    /** Written down in the inbox. */
    ADDED_TO_INBOX,

    /** Written down in the project the user picked. */
    ADDED_TO_PROJECT,

    /** Nothing was written down, and the sheet still holds what was typed. */
    FAILED,
}

/**
 * The capture surface: what it holds, and what happens when it is saved.
 *
 * Written as a plain object driven by a scope rather than as a view model, so
 * the whole of it can be exercised without Compose and without the platform.
 *
 * Saving is optimistic like every other write in the app: the task is inserted
 * into the replica and queued for the server in one transaction, so it appears
 * in the inbox — and anywhere else it belongs — before the device has spoken to
 * anything. The sheet closes on that, not on a response.
 *
 * The installation's two title rules are both read here and they behave
 * differently on purpose: the labels a title earns are attached, and shown
 * beforehand so they can be taken off; the projects a title suggests are only
 * offered, and the task goes to the inbox unless somebody taps one.
 */
class QuickAddPresenter(
    private val scope: CoroutineScope,
    workspace: Flow<QuickAddWorkspace>,
    private val recent: RecentProjects,
    private val actions: QuickAddActions,
    private val zone: ZoneId,
) {
    private val editing = MutableStateFlow(Editing())
    private val outgoing = MutableSharedFlow<QuickAddMessage>(extraBufferCapacity = 4)

    /** What the sheet renders. */
    val state: StateFlow<QuickAddUiState> =
        combine(editing, workspace, recent.observe()) { edit, space, order ->
            render(edit, space, order)
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), QuickAddUiState())

    /** What the sheet says back, once each. */
    val messages: SharedFlow<QuickAddMessage> = outgoing.asSharedFlow()

    /**
     * Opens the sheet on a fresh draft, prefilled with whatever asked for it.
     *
     * Opening always starts from the request rather than from what was left
     * behind, so a share never lands on top of a half-written task from an hour
     * ago. A save that failed is the one thing that survives, because it is
     * still open.
     */
    fun open(request: QuickAddRequest = QuickAddRequest()) {
        editing.value = Editing(visible = true, draft = QuickAddDraft.from(request))
    }

    /** Closes the sheet and forgets the draft. */
    fun dismiss() {
        editing.value = Editing()
    }

    fun setTitles(text: String) = edit { it.copy(titles = text) }

    fun setDescription(text: String) = edit { it.copy(description = text) }

    fun setPriority(priority: Priority) = edit { it.copy(priority = priority) }

    fun setDayPart(dayPart: DayPart) = edit { it.copy(dayPart = dayPart) }

    /** Sets or clears the due date. Tapping the day already set clears it. */
    fun setDueDate(date: LocalDate?) = edit { it.copy(dueDate = if (it.dueDate == date) null else date) }

    /**
     * Replaces the labels the user chose by hand.
     *
     * Choosing a label also un-rejects it: the user has just asked for it,
     * whatever they did to the same suggestion a moment earlier.
     */
    fun setLabels(names: List<String>) =
        edit { draft ->
            draft.copy(labelNames = names, rejectedAutoLabels = draft.rejectedAutoLabels - names.toSet())
        }

    /**
     * Takes a label the title earned back off.
     *
     * The rejection travels with the write, so the server does not re-attach
     * what the user has just removed.
     */
    fun rejectAutoLabel(name: String) =
        edit { draft ->
            if (name in draft.rejectedAutoLabels) {
                draft
            } else {
                draft.copy(rejectedAutoLabels = draft.rejectedAutoLabels + name)
            }
        }

    /** Files the draft into a project, or into the inbox when given `null`. */
    fun chooseProject(projectLocalId: Long?) {
        editing.update { current ->
            current.copy(
                // A project and a column of one are the same decision, so picking
                // a project drops whichever column was chosen before it.
                draft = current.draft.copy(projectLocalId = projectLocalId, sectionLocalId = null),
                pickingProject = false,
                projectQuery = "",
            )
        }
    }

    /** Shows or hides the list of places, clearing whatever was typed into its search box. */
    fun setPickingProject(picking: Boolean) {
        editing.update { it.copy(pickingProject = picking, projectQuery = "") }
    }

    fun setProjectQuery(query: String) {
        editing.update { it.copy(projectQuery = query) }
    }

    /**
     * Writes the draft down: one task per typed line, all of them in the same
     * place and carrying the same fields.
     *
     * A line that was created stays created. If the fifth of five fails, the four
     * before it are already in the replica and queued, so the sheet stays open
     * holding only what is left — resubmitting cannot write the first four twice.
     */
    fun submit() {
        val current = editing.value
        if (current.submitting || !current.draft.canSubmit) return
        editing.value = current.copy(submitting = true)
        scope.launch {
            val draft = current.draft
            val destination = draft.destination()
            val pending = draft.tasks(zone)
            var written = 0
            try {
                for (task in pending) {
                    actions.create(destination, task)
                    written += 1
                }
            } catch (refusal: WriteRefused) {
                stopAfter(draft, pending, written)
                return@launch
            } catch (failure: RuntimeException) {
                stopAfter(draft, pending, written)
                return@launch
            }
            draft.projectLocalId?.let { recent.remember(it) }
            editing.value = Editing()
            outgoing.emit(
                if (draft.isInbox) QuickAddMessage.ADDED_TO_INBOX else QuickAddMessage.ADDED_TO_PROJECT,
            )
        }
    }

    /** Leaves the sheet holding the lines that were not written down, and says so. */
    private suspend fun stopAfter(
        draft: QuickAddDraft,
        pending: List<NewTask>,
        written: Int,
    ) {
        val remaining = pending.drop(written).joinToString("\n") { it.title }
        editing.value = Editing(visible = true, draft = draft.copy(titles = remaining))
        outgoing.emit(QuickAddMessage.FAILED)
    }

    private fun edit(change: (QuickAddDraft) -> QuickAddDraft) {
        editing.update { it.copy(draft = change(it.draft)) }
    }

    private fun render(
        edit: Editing,
        space: QuickAddWorkspace,
        order: List<Long>,
    ): QuickAddUiState {
        val query = edit.projectQuery.trim()
        val offered =
            if (query.isEmpty()) {
                space.projects
            } else {
                space.projects.filter { it.title.contains(query, ignoreCase = true) }
            }
        val leading = pickRecent(order, offered, Project::localId)
        val lifted = leading.mapTo(HashSet()) { it.localId }
        val chosen = edit.draft.projectLocalId
        // Everything already spoken for is kept out of the suggestions: the
        // project the draft points at, and every label already on it or already
        // rejected.
        val spokenFor = edit.draft.labelNames.toSet() + edit.draft.rejectedAutoLabels
        return QuickAddUiState(
            visible = edit.visible,
            draft = edit.draft,
            submitting = edit.submitting,
            pickingProject = edit.pickingProject,
            projectQuery = edit.projectQuery,
            recentProjects = leading,
            otherProjects = offered.filterNot { it.localId in lifted },
            suggestedProjects =
                space.appSettings.suggestedProjects(edit.draft.titles, space.projects, setOfNotNull(chosen)),
            autoLabels = space.appSettings.autoLabelNames(edit.draft.titles, space.labels, spokenFor),
            knownLabels = space.labels,
            projectTitle = space.projects.firstOrNull { it.localId == chosen }?.title,
        )
    }

    /** The part of the sheet's state the user is changing, as one value. */
    private data class Editing(
        val visible: Boolean = false,
        val draft: QuickAddDraft = QuickAddDraft(),
        val submitting: Boolean = false,
        val pickingProject: Boolean = false,
        val projectQuery: String = "",
    )

    private companion object {
        /**
         * How long the queries behind the picker stay open after the surface
         * stops being watched. Matches the lists', so everything on screen stops
         * observing the replica together.
         */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
