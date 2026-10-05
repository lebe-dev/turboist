package ru.tinyops.turboist.core.model

/**
 * A reusable blueprint for a task and its subtasks. [name] doubles as the title
 * of the task created from it.
 *
 * @property position the order templates are offered in.
 */
data class TaskTemplate(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val position: Int = 0,
    val labels: List<Label> = emptyList(),
    val subtasks: List<TaskTemplateSubtask> = emptyList(),
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaEntity

/**
 * One child task captured in a template. Templates are one level deep: a deeper
 * tree is flattened into this list when a template is cut from an existing task.
 *
 * @property createdAt the server stores timestamps on the subtask row but leaves
 *   them out of the template payload, so a subtask rebuilt from the wire carries
 *   its template's lifetime rather than its own.
 */
data class TaskTemplateSubtask(
    override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val templateLocalId: Long = NO_LOCAL_ID,
    val position: Int = 0,
    val title: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val labels: List<Label> = emptyList(),
    val createdAt: Long = 0L,
    val updatedAt: Long = 0L,
) : ReplicaEntity
