package ru.tinyops.turboist.nativeapp.templates

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
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.TaskTemplate
import ru.tinyops.turboist.core.sync.write.TemplateDraft
import ru.tinyops.turboist.core.sync.write.TemplateSubtaskDraft
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler

/**
 * Something the templates screen has to say back after an action.
 *
 * Carried as a value rather than as words, so the screen stays free of resources
 * and of a language. Using a template is the one action with something to say
 * when nothing went wrong: the work it made is in a project the user is not
 * looking at, so without the sentence there is no evidence the gesture landed.
 */
sealed interface TemplateMessage {
    data object Saved : TemplateMessage

    data object SaveFailed : TemplateMessage

    data object Deleted : TemplateMessage

    data object DeleteFailed : TemplateMessage

    /** A template became [taskCount] tasks, the root included. */
    data class Instantiated(val taskCount: Int) : TemplateMessage

    data object InstantiateFailed : TemplateMessage
}

/**
 * A template as the editor has it, before it is saved.
 *
 * The whole structure is held rather than a diff, because saving a template is a
 * replace: the editor submits it whole, so what is being edited is the whole of
 * it.
 *
 * [templateLocalId] is absent while the template is new. Nothing exists yet in
 * that case — the row is written when the editor is saved, not when it opens, so
 * abandoning a half-typed template leaves nothing behind.
 */
data class TemplateEditor(
    val templateLocalId: Long? = null,
    val name: String = "",
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val labelLocalIds: List<Long> = emptyList(),
    val lines: List<TemplateSubtaskDraft> = emptyList(),
) {
    /** A template is named by the task it makes, so an unnamed one cannot be saved. */
    val canSave: Boolean get() = name.isNotBlank()

    /** What the editor would save, with blank lines dropped as the server drops them. */
    fun asDraft(): TemplateDraft =
        TemplateDraft(
            name = name.trim(),
            description = description.trim(),
            priority = priority,
            dayPart = dayPart,
            labelLocalIds = labelLocalIds,
            subtasks =
                lines.mapNotNull { line ->
                    line.title.trim().takeIf { it.isNotEmpty() }?.let { line.copy(title = it) }
                },
        )
}

/** Everything the templates screen renders. */
data class TemplatesUiState(
    val loading: Boolean = true,
    val templates: List<TaskTemplate> = emptyList(),
    /** The projects a template can be dropped into. */
    val projects: List<Project> = emptyList(),
    /** The labels a template can name. */
    val knownLabels: List<Label> = emptyList(),
    /** The template being written, while the editor is open. */
    val editor: TemplateEditor? = null,
    /** The template waiting for a project to be picked, while that picker is open. */
    val choosingProjectFor: TaskTemplate? = null,
    val refreshing: Boolean = false,
) {
    /** True once the workspace is known to hold no templates, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && templates.isEmpty()
}

/**
 * The behaviour of the templates screen.
 *
 * A plain object driven by a scope, like every other presenter here, so all of
 * it runs without Compose and without the platform.
 *
 * The editor lives in this state rather than in the screen's own remembered
 * values on purpose: what is being written is a whole structure with lines that
 * can be added, retitled and taken out again, and a rotation must not lose it.
 */
class TemplatesPresenter(
    private val scope: CoroutineScope,
    templates: Flow<List<TaskTemplate>>,
    projects: Flow<List<Project>>,
    knownLabels: Flow<List<Label>>,
    private val actions: TemplateActions,
    private val sync: SyncScheduler,
) {
    private val editor = MutableStateFlow<TemplateEditor?>(null)
    private val choosing = MutableStateFlow<TaskTemplate?>(null)
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<TemplateMessage>(extraBufferCapacity = 2)

    /**
     * The task a template just became, once each.
     *
     * Emitted so the screen can offer to open the work rather than only saying it
     * exists: the tasks were made somewhere the user is not looking.
     */
    private val opened = MutableSharedFlow<Long>(extraBufferCapacity = 1)

    /** What the screen renders. */
    val state: StateFlow<TemplatesUiState> =
        combine(templates, projects, knownLabels, editor, choosing) { all, known, labels, editing, picking ->
            TemplatesUiState(
                loading = false,
                templates = all,
                projects = known,
                knownLabels = labels,
                editor = editing,
                // The picker follows the template it was opened for, so a
                // template deleted on another device closes it instead of
                // leaving a dialog with nothing behind it.
                choosingProjectFor = picking?.let { chosen -> all.firstOrNull { it.localId == chosen.localId } },
            )
        }.combine(refreshing) { rendered, isRefreshing -> rendered.copy(refreshing = isRefreshing) }
            .stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), TemplatesUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<TemplateMessage> = outgoing.asSharedFlow()

    /** The tasks a template made, for a screen that offers to open them. */
    val instantiated: SharedFlow<Long> = opened.asSharedFlow()

    /** Asks the sync engine to catch up. The list is a query and needs no reload. */
    fun refresh() {
        scope.launch {
            refreshing.value = true
            try {
                sync.requestSyncNow()
            } finally {
                refreshing.value = false
            }
        }
    }

    // --- the editor ----------------------------------------------------------

    /** Opens the editor on a new template. */
    fun startNewTemplate() {
        editor.value = TemplateEditor()
    }

    /** Opens the editor on a template that exists, filled in with what it holds. */
    fun edit(template: TaskTemplate) {
        editor.value =
            TemplateEditor(
                templateLocalId = template.localId,
                name = template.name,
                description = template.description,
                priority = template.priority,
                dayPart = template.dayPart,
                labelLocalIds = template.labels.map { it.localId },
                lines =
                    template.subtasks.map { line ->
                        TemplateSubtaskDraft(
                            title = line.title,
                            description = line.description,
                            priority = line.priority,
                            dayPart = line.dayPart,
                            labelLocalIds = line.labels.map { it.localId },
                        )
                    },
            )
    }

    /** Opens the editor on a draft cut from an existing task, ready to be saved. */
    fun draftFromTask(taskLocalId: Long) {
        scope.launch {
            val draft =
                try {
                    actions.draftFromTask(taskLocalId)
                } catch (refusal: WriteRefused) {
                    outgoing.emit(TemplateMessage.SaveFailed)
                    return@launch
                } catch (failure: RuntimeException) {
                    outgoing.emit(TemplateMessage.SaveFailed)
                    return@launch
                }
            editor.value =
                TemplateEditor(
                    name = draft.name,
                    description = draft.description,
                    priority = draft.priority,
                    dayPart = draft.dayPart,
                    labelLocalIds = draft.labelLocalIds,
                    lines = draft.subtasks,
                )
        }
    }

    /** Closes the editor, discarding whatever was being written. */
    fun cancelEdit() {
        editor.value = null
    }

    fun rename(name: String) = editEditor { it.copy(name = name) }

    fun describe(description: String) = editEditor { it.copy(description = description) }

    fun setPriority(priority: Priority) = editEditor { it.copy(priority = priority) }

    fun setDayPart(dayPart: DayPart) = editEditor { it.copy(dayPart = dayPart) }

    /** Puts a label on the template being written, or takes it off again. */
    fun toggleLabel(labelLocalId: Long) =
        editEditor { draft ->
            val chosen = draft.labelLocalIds
            draft.copy(
                labelLocalIds = if (labelLocalId in chosen) chosen - labelLocalId else chosen + labelLocalId,
            )
        }

    /** Adds an empty line to the checklist, for the user to type into. */
    fun addLine() = editEditor { it.copy(lines = it.lines + TemplateSubtaskDraft(title = "")) }

    fun retitleLine(
        index: Int,
        title: String,
    ) = editEditor { draft ->
        if (index !in draft.lines.indices) return@editEditor draft
        draft.copy(
            lines = draft.lines.mapIndexed { at, line -> if (at == index) line.copy(title = title) else line },
        )
    }

    fun removeLine(index: Int) =
        editEditor { draft ->
            if (index !in draft.lines.indices) return@editEditor draft
            draft.copy(lines = draft.lines.filterIndexed { at, _ -> at != index })
        }

    /**
     * Saves what the editor holds, as a new template or over the one it opened.
     *
     * An unnamed template is not saved and the editor stays open: the name is the
     * title of every task the template will make, and the server refuses one
     * without it.
     */
    fun save() {
        val draft = editor.value ?: return
        if (!draft.canSave) return
        scope.launch {
            val saved =
                report(TemplateMessage.SaveFailed) {
                    val templateLocalId = draft.templateLocalId
                    if (templateLocalId == null) {
                        actions.create(draft.asDraft())
                    } else {
                        actions.replace(templateLocalId, draft.asDraft())
                    }
                }
            if (!saved) return@launch
            editor.value = null
            outgoing.emit(TemplateMessage.Saved)
        }
    }

    // --- using and removing templates ---------------------------------------

    fun delete(template: TaskTemplate) {
        scope.launch {
            if (report(TemplateMessage.DeleteFailed) { actions.delete(template.localId) }) {
                outgoing.emit(TemplateMessage.Deleted)
            }
        }
    }

    /** Opens the project picker for a template. */
    fun startInstantiate(template: TaskTemplate) {
        choosing.value = template
    }

    /** Closes the project picker without using the template. */
    fun cancelInstantiate() {
        choosing.value = null
    }

    /**
     * Turns the chosen template into work in a project.
     *
     * The tasks appear at once and carry no server id until the queue drains,
     * which is why the count is reported from what the template holds rather than
     * from anything the server said.
     */
    fun instantiate(projectLocalId: Long) {
        val template = choosing.value ?: return
        scope.launch {
            var rootLocalId: Long? = null
            val made =
                report(TemplateMessage.InstantiateFailed) {
                    rootLocalId = actions.instantiate(template.localId, projectLocalId)
                }
            if (!made) return@launch
            choosing.value = null
            outgoing.emit(TemplateMessage.Instantiated(template.subtasks.size + 1))
            rootLocalId?.let { opened.emit(it) }
        }
    }

    // --- internals -----------------------------------------------------------

    private fun editEditor(change: (TemplateEditor) -> TemplateEditor) {
        editor.value = editor.value?.let(change)
    }

    /**
     * Runs a write and says so when it did not happen.
     *
     * Nothing a template write can be refused for is something the user can act
     * on — a template names no other row and takes up no capacity — so there is
     * one sentence for all of them rather than a false distinction.
     */
    private suspend fun report(
        onFailure: TemplateMessage,
        write: suspend () -> Unit,
    ): Boolean {
        try {
            write()
            return true
        } catch (refusal: WriteRefused) {
            outgoing.emit(onFailure)
        } catch (failure: RuntimeException) {
            outgoing.emit(onFailure)
        }
        return false
    }

    private companion object {
        /** Matches the other presenters, so a screen and the queries behind it stop together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
