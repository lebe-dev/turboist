package ru.tinyops.turboist.core.database.entity

import androidx.room.Entity
import androidx.room.ForeignKey
import androidx.room.Index
import androidx.room.PrimaryKey
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory

/**
 * A task.
 *
 * Placement is exclusive on the server — a task sits in the inbox, in a context,
 * in a project, or in a section of one — but the replica does not re-state that
 * as constraints. The rule is enforced once, in the service layer the writes go
 * through; a second copy on the device could only ever disagree with it, and a
 * disagreement would reject an authoritative row the server already accepted.
 * What the replica does enforce is referential integrity, because a dangling
 * reference is not a policy question, it is a broken row.
 *
 * [inboxId] is the one placement field that is not a local id: the inbox is a
 * single fixed server row rather than a replicated entity, so its id is the same
 * number on every device and needs no resolution.
 *
 * References are checked as each statement runs, not at the end of the
 * transaction. Whoever applies a batch of records therefore has to write the
 * rows a task points at before the task itself. That is a real obligation, and
 * it is the cheaper one: a reference that would dangle is refused on the
 * statement that made it, naming the row, instead of surfacing as one opaque
 * failure when a whole batch is committed.
 */
@Entity(
    tableName = "tasks",
    indices = [
        Index(value = ["serverId"], unique = true),
        Index(value = ["contextLocalId"]),
        Index(value = ["projectLocalId"]),
        Index(value = ["sectionLocalId"]),
        Index(value = ["parentLocalId"]),
        Index(value = ["sourceTaskLocalId"]),
        Index(value = ["planState"]),
        Index(value = ["deadlineAt"]),
        // The columns the workspace-wide lists filter on. Each is empty on
        // most rows, so an index over one answers its list by touching only
        // the handful of rows that belong to it instead of reading every task
        // the device holds.
        Index(value = ["inboxId"]),
        Index(value = ["isPinned"]),
        Index(value = ["troikiCategory"]),
        // The dated lists ask two questions at once — does this date fall
        // inside the window, and is the work still open — and the date is the
        // narrow half: a day holds a few tasks while nearly every task on the
        // device is open. So the date leads and the status rides along behind
        // it, which settles the second question from the index without reading
        // a row. For the completion history the same index supplies the order
        // too, so a page of it is read straight off the index instead of being
        // cut from a sorted copy of the whole history.
        //
        // Deliberately in this order, and deliberately without a plain index
        // on the status column. An index that leads with the status looks
        // cheapest to the engine for any list that mentions it — the engine
        // has no statistics and reads an equality test as narrow — so it wins
        // over the selective index every other list of open work depends on,
        // and the backlog, the daily plan and the inbox each end up reading
        // every open task and filtering row by row. Keeping the status out of
        // every leading position is what leaves each list on its own column.
        Index(value = ["dueAt", "status"]),
        Index(value = ["completedAt", "status"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = ContextRow::class,
            parentColumns = ["localId"],
            childColumns = ["contextLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        ForeignKey(
            entity = ProjectRow::class,
            parentColumns = ["localId"],
            childColumns = ["projectLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        // Deleting a board column keeps its tasks in the project, unfiled.
        ForeignKey(
            entity = ProjectSectionRow::class,
            parentColumns = ["localId"],
            childColumns = ["sectionLocalId"],
            onDelete = ForeignKey.SET_NULL,
        ),
        ForeignKey(
            entity = TaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["parentLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        // A recurrence snapshot outlives the recurring task it was cut from; it
        // only loses the pointer back to it.
        ForeignKey(
            entity = TaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["sourceTaskLocalId"],
            onDelete = ForeignKey.SET_NULL,
        ),
    ],
)
data class TaskRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val title: String,
    val description: String = "",
    val inboxId: Long? = null,
    val contextLocalId: Long? = null,
    val projectLocalId: Long? = null,
    val sectionLocalId: Long? = null,
    val parentLocalId: Long? = null,
    val priority: Priority = Priority.NONE,
    val status: TaskStatus = TaskStatus.OPEN,
    val dueAt: Long? = null,
    val dueHasTime: Boolean = false,
    val deadlineAt: Long? = null,
    val deadlineHasTime: Boolean = false,
    val dayPart: DayPart = DayPart.NONE,
    val planState: PlanState = PlanState.NONE,
    val isPinned: Boolean = false,
    val pinnedAt: Long? = null,
    val isPrivate: Boolean = false,
    val isComplex: Boolean = false,
    val completedAt: Long? = null,
    val recurrenceRule: String? = null,
    val sourceTaskLocalId: Long? = null,
    val postponeCount: Int = 0,
    val troikiCategory: TroikiCategory? = null,
    val createdAt: Long,
    val updatedAt: Long,
) : ReplicaRow<TaskRow> {
    override fun withLocalId(localId: Long): TaskRow = copy(localId = localId)
}

/**
 * A label attached to a task.
 *
 * [createdAt] is the moment the label was applied, not the moment the task was
 * created — re-tagging during a weekly review is exactly what the usage stats
 * are asked about. It is nullable because the server's own column is: rows that
 * predate the timestamp carry an approximation, and a row that somehow has none
 * counts as outside every window rather than as tagged at the epoch.
 */
@Entity(
    tableName = "task_labels",
    primaryKeys = ["taskLocalId", "labelLocalId"],
    indices = [
        Index(value = ["labelLocalId"]),
        Index(value = ["createdAt"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = TaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["taskLocalId"],
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
data class TaskLabelRow(
    val taskLocalId: Long,
    val labelLocalId: Long,
    val createdAt: Long? = null,
)

/**
 * One directed edge of the task graph.
 *
 * A `blocks` edge is enforced: its target cannot be completed while its source
 * is still open. A `related` edge is symmetric and informational; the server
 * normalises such a pair so the lower id is the source, and the replica stores
 * whatever it is told rather than re-deciding.
 *
 * The unique index is the server's constraint mirrored: the same pair of tasks
 * carries at most one edge of each kind, so replaying a change that has already
 * been applied cannot double the graph.
 */
@Entity(
    tableName = "task_relations",
    indices = [
        Index(value = ["serverId"], unique = true),
        Index(value = ["sourceTaskLocalId", "targetTaskLocalId", "type"], unique = true),
        Index(value = ["targetTaskLocalId"]),
    ],
    foreignKeys = [
        ForeignKey(
            entity = TaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["sourceTaskLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
        ForeignKey(
            entity = TaskRow::class,
            parentColumns = ["localId"],
            childColumns = ["targetTaskLocalId"],
            onDelete = ForeignKey.CASCADE,
        ),
    ],
)
data class TaskRelationRow(
    @PrimaryKey(autoGenerate = true) override val localId: Long = NO_LOCAL_ID,
    override val serverId: Long? = null,
    val sourceTaskLocalId: Long,
    val targetTaskLocalId: Long,
    val type: RelationType,
    val createdAt: Long,
) : ReplicaRow<TaskRelationRow> {
    override fun withLocalId(localId: Long): TaskRelationRow = copy(localId = localId)
}
