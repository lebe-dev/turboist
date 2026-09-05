package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.Serializable

/**
 * Wire shapes of the replicated entities.
 *
 * Every id in here is a **server** id, and every timestamp is the API's UTC
 * millisecond form. Turning either into what the replica stores is the job of the
 * mapping functions, never of these types: a DTO stays a faithful transcript of
 * what came off the wire, so a decoding problem can always be told apart from a
 * mapping one.
 *
 * Fields carry defaults so that a payload written by a server older than this
 * build still decodes.
 */
@Serializable
data class LabelDto(
    val id: Long = 0,
    val name: String = "",
    val color: String = "",
    val isFavourite: Boolean = false,
    val isPrivate: Boolean = false,
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class ContextDto(
    val id: Long = 0,
    val name: String = "",
    val color: String = "",
    val isFavourite: Boolean = false,
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class ProjectDto(
    val id: Long = 0,
    val contextId: Long = 0,
    val title: String = "",
    val description: String = "",
    val color: String = "",
    val status: String = "",
    val projectType: String = "",
    val isPinned: Boolean = false,
    val pinnedAt: String? = null,
    val isPrivate: Boolean = false,
    val troikiCategory: String? = null,
    val labels: List<LabelDto> = emptyList(),
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

@Serializable
data class SectionDto(
    val id: Long = 0,
    val projectId: Long = 0,
    val title: String = "",
    val position: Int = 0,
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

/**
 * A task.
 *
 * @property blockedByCount how many still-open tasks block this one, counting
 *   those inherited from its ancestors. Present on every read path, because it is
 *   what decides whether the checkbox may be ticked at all.
 * @property relationCount the task's own relations, both kinds and directions.
 * @property relations the edges themselves, sent only where they are asked for.
 * @property subtasks sent only by the single-task read that asks for them.
 */
@Serializable
data class TaskDto(
    val id: Long = 0,
    val title: String = "",
    val description: String = "",
    val inboxId: Long? = null,
    val contextId: Long? = null,
    val projectId: Long? = null,
    val sectionId: Long? = null,
    val parentId: Long? = null,
    val priority: String = "",
    val status: String = "",
    val dueAt: String? = null,
    val dueHasTime: Boolean = false,
    val deadlineAt: String? = null,
    val deadlineHasTime: Boolean = false,
    val dayPart: String = "",
    val planState: String = "",
    val isPinned: Boolean = false,
    val pinnedAt: String? = null,
    val isPrivate: Boolean = false,
    val isComplex: Boolean = false,
    val completedAt: String? = null,
    val recurrenceRule: String? = null,
    val sourceTaskId: Long? = null,
    val postponeCount: Int = 0,
    val labels: List<LabelDto> = emptyList(),
    val url: String = "",
    val createdAt: String? = null,
    val updatedAt: String? = null,
    val parentTitle: String? = null,
    val blockedByCount: Int = 0,
    val relationCount: Int = 0,
    val relations: List<TaskRelationDto> = emptyList(),
    val subtasks: PageDto<TaskDto>? = null,
)

/**
 * A relation as seen from the task it was read for: `direction` describes the
 * peer end, so the same stored row reads as "blocks" on one side and "blocked by"
 * on the other. Only meaningful for a blocking relation.
 */
@Serializable
data class TaskRelationDto(
    val id: Long = 0,
    val type: String = "",
    val direction: String = "",
    val task: TaskDto = TaskDto(),
    val createdAt: String? = null,
)

/**
 * The stored relation row itself, naming both of its endpoints. This is the form
 * a client holding the whole graph wants: it derives the direction locally for
 * whichever end it happens to be drawing.
 */
@Serializable
data class TaskRelationEdgeDto(
    val id: Long = 0,
    val sourceTaskId: Long = 0,
    val targetTaskId: Long = 0,
    val type: String = "",
    val createdAt: String? = null,
)

@Serializable
data class TaskTemplateSubtaskDto(
    val id: Long = 0,
    val title: String = "",
    val description: String = "",
    val priority: String = "",
    val dayPart: String = "",
    val labels: List<LabelDto> = emptyList(),
)

@Serializable
data class TaskTemplateDto(
    val id: Long = 0,
    val name: String = "",
    val description: String = "",
    val priority: String = "",
    val dayPart: String = "",
    val position: Int = 0,
    val labels: List<LabelDto> = emptyList(),
    val subtasks: List<TaskTemplateSubtaskDto> = emptyList(),
    val createdAt: String? = null,
    val updatedAt: String? = null,
)

/**
 * The user's own preferences.
 *
 * `bannerDayPart` is an empty string for "all day" rather than the `none` day
 * part, which is why it is not simply a day-part enum on the wire.
 */
@Serializable
data class UserSettingsDto(
    val weeklyUnplannedExcludedLabelIds: List<Long> = emptyList(),
    val bugLabelIds: List<Long> = emptyList(),
    val locale: String = "",
    val publicView: Boolean = false,
    val bannerText: String = "",
    val bannerPublished: Boolean = false,
    val bannerDayPart: String = "",
    val calendarEnabled: Boolean = false,
    val calendarHidePastEvents: Boolean = true,
    val troikiEnabled: Boolean = false,
    val maxPinnedTasks: Int = 0,
    val maxPinnedProjects: Int = 0,
)

@Serializable
data class AutoLabelRuleDto(
    val mask: String = "",
    val labelIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

@Serializable
data class ProjectSuggestionRuleDto(
    val mask: String = "",
    val projectIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
)

/** Installation-wide rules, distinct from the per-user preferences above. */
@Serializable
data class AppSettingsDto(
    val autoLabels: List<AutoLabelRuleDto> = emptyList(),
    val projectSuggestions: List<ProjectSuggestionRuleDto> = emptyList(),
)

/** One entity of the two-slot jump pair, hydrated with the title for display. */
@Serializable
data class HarpoonSlotDto(
    val kind: String = "",
    val id: Long = 0,
    val title: String = "",
)

@Serializable
data class HarpoonDto(
    val slots: List<HarpoonSlotDto> = emptyList(),
)
