package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable
import ru.tinyops.turboist.core.network.Clearable

/**
 * Request bodies for the mutations.
 *
 * Two conventions run through all of them, and both come from how the API reads a
 * body:
 *
 * - **Optional means nullable-with-a-null-default.** Nulls are not written, so a
 *   field left alone never appears in the body and the server leaves it alone
 *   too. That is what makes a PATCH a patch rather than a full overwrite.
 * - **Emptying a field is [Clearable.Clear]**, not `null`. `null` is "not
 *   touching it"; only an explicit JSON null clears a due date or a recurrence,
 *   and that is the difference the [Clearable] wrapper exists to express.
 *
 * Label fields are lists of label *names* on create and patch — the server
 * resolves or creates them — while the rule and template payloads carry label
 * ids, which is what those endpoints accept.
 */
@Serializable
data class CreateTaskRequest(
    val title: String,
    val description: String? = null,
    val priority: String? = null,
    val dueAt: String? = null,
    val dueHasTime: Boolean? = null,
    val deadlineAt: String? = null,
    val deadlineHasTime: Boolean? = null,
    val dayPart: String? = null,
    val planState: String? = null,
    val recurrenceRule: String? = null,
    val labels: List<String>? = null,
    /**
     * Label names an installation rule would have attached automatically and the
     * user took off again. Sent so the rule does not put them straight back.
     */
    val removedAutoLabels: List<String>? = null,
)

@Serializable
data class PatchTaskRequest(
    val title: String? = null,
    val description: String? = null,
    val priority: String? = null,
    val dueAt: Clearable<String>? = null,
    val dueHasTime: Boolean? = null,
    val deadlineAt: Clearable<String>? = null,
    val deadlineHasTime: Boolean? = null,
    val dayPart: String? = null,
    val planState: String? = null,
    val recurrenceRule: Clearable<String>? = null,
    val labels: List<String>? = null,
    val removedAutoLabels: List<String>? = null,
    val isPrivate: Boolean? = null,
    val isComplex: Boolean? = null,
)

/**
 * Where a task goes. Placement is exclusive: exactly one destination is set, and
 * a parent id makes the task a subtask of another one.
 */
@Serializable
data class MoveTaskRequest(
    val inboxId: Long? = null,
    val contextId: Long? = null,
    val projectId: Long? = null,
    val sectionId: Long? = null,
    val parentId: Long? = null,
)

/** Commits a task to the current week or parks it in the backlog; the server cascades to open subtasks. */
@Serializable
data class PlanTaskRequest(
    val state: String,
)

/**
 * Completing a task.
 *
 * [completedAt] exists so a write made without a network connection is recorded
 * at the moment the user actually ticked it, not at the moment the queue happened
 * to drain — which is what keeps a day's completed history honest.
 */
@Serializable
data class CompleteTaskRequest(
    val completedAt: String? = null,
)

@Serializable
data class CreateTaskRelationRequest(
    val targetTaskId: Long,
    val type: String,
    /** Read relative to the task in the path, and ignored for a symmetric relation. */
    val direction: String? = null,
)

@Serializable
data class DecomposeTaskRequest(
    val titles: List<String>,
)

@Serializable
data class DecomposeTaskResponse(
    val created: List<TaskDto> = emptyList(),
)

@Serializable
data class BulkIdsRequest(
    val ids: List<Long>,
)

@Serializable
data class BulkMoveRequest(
    val ids: List<Long>,
    val inboxId: Long? = null,
    val contextId: Long? = null,
    val projectId: Long? = null,
    val sectionId: Long? = null,
    val parentId: Long? = null,
)

@Serializable
data class BulkPriorityRequest(
    val ids: List<Long>,
    val priority: String,
)

/**
 * A bulk call answers per item rather than all-or-nothing: one task refused
 * because something still blocks it must not undo the nine that went through.
 */
@Serializable
data class BulkResultDto(
    val succeeded: List<Long> = emptyList(),
    val failed: List<BulkFailureDto> = emptyList(),
)

@Serializable
data class BulkFailureDto(
    val id: Long = 0,
    val error: BulkFailureReasonDto = BulkFailureReasonDto(),
)

@Serializable
data class BulkFailureReasonDto(
    val code: String = "",
    val message: String = "",
)

/** Creates a parent task and re-parents the listed tasks under it. */
@Serializable
data class GroupTasksRequest(
    val title: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val planState: String? = null,
    val labels: List<String>? = null,
    val projectId: Long? = null,
    val sectionId: Long? = null,
    val contextId: Long? = null,
    val childIds: List<Long> = emptyList(),
)

@Serializable
data class GroupTasksResponse(
    val parent: TaskDto = TaskDto(),
    val succeeded: List<Long> = emptyList(),
    val failed: List<BulkFailureDto> = emptyList(),
)

@Serializable
data class CreateProjectRequest(
    val title: String,
    val description: String? = null,
    val color: String? = null,
    val labels: List<String>? = null,
    val projectType: String? = null,
)

@Serializable
data class PatchProjectRequest(
    val title: String? = null,
    val description: String? = null,
    val color: String? = null,
    val contextId: Long? = null,
    val labels: List<String>? = null,
    val isPrivate: Boolean? = null,
    val projectType: String? = null,
)

/**
 * Puts a project in one of the daily plan's three slots.
 *
 * An absent category takes it out of all of them: nulls are not written, so
 * "clear it" and "no category" are the same body, which is exactly how the
 * endpoint reads it.
 */
@Serializable
data class SetTroikiCategoryRequest(
    val category: String? = null,
)

@Serializable
data class CreateSectionRequest(
    val title: String,
)

@Serializable
data class PatchSectionRequest(
    val title: String? = null,
)

/** Moves a section to a new left-to-right position on its board. */
@Serializable
data class ReorderSectionRequest(
    val position: Int,
)

@Serializable
data class CreateContextRequest(
    val name: String,
    val color: String? = null,
    val isFavourite: Boolean? = null,
)

@Serializable
data class PatchContextRequest(
    val name: String? = null,
    val color: String? = null,
    val isFavourite: Boolean? = null,
)

@Serializable
data class CreateLabelRequest(
    val name: String,
    val color: String? = null,
    val isFavourite: Boolean? = null,
)

@Serializable
data class PatchLabelRequest(
    val name: String? = null,
    val color: String? = null,
    val isFavourite: Boolean? = null,
    val isPrivate: Boolean? = null,
)

@Serializable
data class TemplateSubtaskRequest(
    val title: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val labelIds: List<Long> = emptyList(),
)

/**
 * Creating or editing a template. The edit is a full replace rather than a patch:
 * the editor always submits the whole structure, and a partial update of a nested
 * subtask list has no sane meaning.
 */
@Serializable
data class TaskTemplateRequest(
    val name: String,
    val description: String? = null,
    val priority: String? = null,
    val dayPart: String? = null,
    val labelIds: List<Long> = emptyList(),
    val subtasks: List<TemplateSubtaskRequest> = emptyList(),
)

@Serializable
data class InstantiateTemplateRequest(
    val projectId: Long,
)

@Serializable
data class InstantiateTemplateResponse(
    val root: TaskDto = TaskDto(),
    val subtasks: List<TaskDto> = emptyList(),
)

@Serializable
data class PatchUserSettingsRequest(
    val weeklyUnplannedExcludedLabelIds: List<Long>? = null,
    val bugLabelIds: List<Long>? = null,
    val locale: String? = null,
    val publicView: Boolean? = null,
    val bannerText: String? = null,
    val bannerPublished: Boolean? = null,
    /** An empty string means "all day"; the three real phases narrow the banner. */
    val bannerDayPart: String? = null,
    val calendarEnabled: Boolean? = null,
    val calendarHidePastEvents: Boolean? = null,
    val troikiEnabled: Boolean? = null,
    val maxPinnedTasks: Int? = null,
    val maxPinnedProjects: Int? = null,
)

@Serializable
data class AutoLabelsRequest(
    val autoLabels: List<AutoLabelRuleDto> = emptyList(),
)

@Serializable
data class ProjectSuggestionsRequest(
    val projectSuggestions: List<ProjectSuggestionRuleDto> = emptyList(),
)

@Serializable
data class HarpoonRefRequest(
    val kind: String,
    val id: Long,
)
