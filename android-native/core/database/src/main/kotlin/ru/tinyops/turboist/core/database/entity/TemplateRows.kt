package ru.tinyops.turboist.core.database.entity

import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Priority

/**
 * A reusable blueprint for a task and its subtasks.
 *
 * A template is one level deep: a deeper tree is flattened into the subtask list
 * when the template is cut from an existing task. That is why the subtask row
 * has no parent of its own.
 */
@Entity(
    tableName = "task_templates",
    indices = [Index(value = ["serverId"], unique = true)],
)
data class TaskTemplateRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val name: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val position: Int = 0,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<TaskTemplateRow> {
    override fun withLocalId(localId: Long): TaskTemplateRow = copy(localId = localId)
}

@Entity(
    tableName = "task_template_subtasks",
    indices = [
        Index(value = ["serverId"], unique = true),
        Index(value = ["templateLocalId", "position"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = TaskTemplateRow::class,
            parentColumns = ["localId"],
            childColumns = ["templateLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class TaskTemplateSubtaskRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val templateLocalId: Long,
    val position: Int = 0,
    val title: String,
    val description: String = "",
    val priority: Priority = Priority.NONE,
    val dayPart: DayPart = DayPart.NONE,
    val createdAt: Long = 0L,
    val updatedAt: Long = 0L,
) : ReplicaRow<TaskTemplateSubtaskRow> {
    override fun withLocalId(localId: Long): TaskTemplateSubtaskRow = copy(localId = localId)
}

@Entity(
    tableName = "task_template_labels",
    primaryKeys = ["templateLocalId", "labelLocalId"],
    indices = [Index(value = ["labelLocalId"])],
    foreignKeys = [
        ForeignKey(
            entity = TaskTemplateRow::class,
            parentColumns = ["localId"],
            childColumns = ["templateLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        ForeignKey(
            entity = LabelRow::class,
            parentColumns = ["localId"],
            childColumns = ["labelLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class TaskTemplateLabelRow(
    val templateLocalId: Long,
    val labelLocalId: Long,
)

@Entity(
    tableName = "task_template_subtask_labels",
    primaryKeys = ["subtaskLocalId", "labelLocalId"],
    indices = [Index(value = ["labelLocalId"])],
    foreignKeys = [
        ForeignKey(
            entity = TaskTemplateSubtaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["subtaskLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        ForeignKey(
            entity = LabelRow::class,
            parentColumns = ["localId"],
            childColumns = ["labelLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class TaskTemplateSubtaskLabelRow(
    val subtaskLocalId: Long,
    val labelLocalId: Long,
)
