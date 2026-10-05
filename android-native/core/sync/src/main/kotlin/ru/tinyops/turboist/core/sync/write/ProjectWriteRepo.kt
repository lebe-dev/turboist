package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.network.dto.CreateProjectRequest
import ru.tinyops.turboist.core.network.dto.CreateSectionRequest
import ru.tinyops.turboist.core.network.dto.PatchSectionRequest

/** A project the user is starting. */
data class NewProject(
    val title: String,
    val description: String = "",
    val color: String = "",
    val labels: List<String> = emptyList(),
    val type: ProjectType = ProjectType.GENERIC,
)

/** An edit to a project. Absent means untouched, exactly as it does for a task. */
data class ProjectEdit(
    val title: String? = null,
    val description: String? = null,
    val color: String? = null,
    val contextLocalId: Long? = null,
    val labels: List<String>? = null,
    val isPrivate: Boolean? = null,
    val type: ProjectType? = null,
)

/**
 * Every write a screen can make to a project or to a column of its board.
 *
 * Board positions are written optimistically and are the one thing here that is
 * expected to be corrected: the server renumbers a board when a column moves, and
 * the device only moves the one column the user dragged. The board looks right
 * immediately and is right after the next pull.
 */
class ProjectWriteRepo(
    private val db: TurboistDatabase,
    private val writer: OutboxWriter,
    private val rules: ReplicaRules = ReplicaRules(db),
) {
    suspend fun create(
        contextLocalId: Long,
        project: NewProject,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            db.contexts().byLocalId(contextLocalId)
                ?: throw WriteRefused.RowMissing("context", contextLocalId)
            val localId =
                db.projects().insert(
                    ProjectRow(
                        contextLocalId = contextLocalId,
                        title = project.title,
                        description = project.description,
                        color = project.color,
                        type = project.type,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            db.projects().setLabels(localId, resolveLabels(project.labels))
            val opId =
                writer.enqueue(
                    CreateProjectOp(
                        projectLocalId = localId,
                        contextLocalId = contextLocalId,
                        body =
                            CreateProjectRequest(
                                title = project.title,
                                description = project.description.ifEmpty { null },
                                color = project.color.ifEmpty { null },
                                labels = project.labels.ifEmpty { null },
                                projectType = project.type.wire.takeIf { project.type != ProjectType.GENERIC },
                            ),
                    ),
                    ReplicaEntityKind.PROJECT,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    suspend fun patch(
        projectLocalId: Long,
        edit: ProjectEdit,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireProject(projectLocalId)
            db.projects().update(
                row.copy(
                    title = edit.title ?: row.title,
                    description = edit.description ?: row.description,
                    color = edit.color ?: row.color,
                    contextLocalId = edit.contextLocalId ?: row.contextLocalId,
                    isPrivate = edit.isPrivate ?: row.isPrivate,
                    type = edit.type ?: row.type,
                    updatedAt = at,
                ),
            )
            edit.labels?.let { db.projects().setLabels(projectLocalId, resolveLabels(it)) }
            val opId =
                writer.enqueue(
                    PatchProjectOp(
                        projectLocalId = projectLocalId,
                        title = edit.title,
                        description = edit.description,
                        color = edit.color,
                        contextLocalId = edit.contextLocalId,
                        labels = edit.labels,
                        isPrivate = edit.isPrivate,
                        projectType = edit.type?.wire,
                    ),
                    ReplicaEntityKind.PROJECT,
                    projectLocalId,
                )
            QueuedWrite(opId, projectLocalId)
        }

    /**
     * Removes a project, and with it everything filed under it — the same cascade
     * the server performs, because hard deletes are the only kind this product
     * has on either side.
     */
    suspend fun delete(projectLocalId: Long): QueuedWrite =
        writer.transaction {
            val row = requireProject(projectLocalId)
            val opId =
                writer.enqueue(
                    DeleteProjectOp(projectLocalId, row.serverId),
                    ReplicaEntityKind.PROJECT,
                    projectLocalId,
                )
            db.projects().delete(row)
            QueuedWrite(opId, projectLocalId)
        }

    /** Finishes, reopens, abandons, files away or brings back a project. */
    suspend fun setStatus(
        projectLocalId: Long,
        action: ProjectStatusAction,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireProject(projectLocalId)
            db.projects().update(row.copy(status = statusAfter(action), updatedAt = at))
            val opId =
                writer.enqueue(
                    ProjectStatusOp(projectLocalId, action),
                    ReplicaEntityKind.PROJECT,
                    projectLocalId,
                )
            QueuedWrite(opId, projectLocalId)
        }

    suspend fun pin(projectLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireProject(projectLocalId)
            if (!row.isPinned) rules.assertRoomToPinProject()
            db.projects().update(row.copy(isPinned = true, pinnedAt = at, updatedAt = at))
            val opId = writer.enqueue(PinProjectOp(projectLocalId), ReplicaEntityKind.PROJECT, projectLocalId)
            QueuedWrite(opId, projectLocalId)
        }

    suspend fun unpin(projectLocalId: Long): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireProject(projectLocalId)
            db.projects().update(row.copy(isPinned = false, pinnedAt = null, updatedAt = at))
            val opId =
                writer.enqueue(UnpinProjectOp(projectLocalId), ReplicaEntityKind.PROJECT, projectLocalId)
            QueuedWrite(opId, projectLocalId)
        }

    /**
     * Puts a project into one of the three daily slots, or takes it out of all of
     * them.
     *
     * Only an open project can take a slot: the daily plan is about work in
     * progress, and a finished one occupying a place would be a place nobody
     * could use.
     *
     * Taking a slot fixes the priority of everything open in the project, so the
     * tasks are re-pinned here as well. Leaving that to the server would show the
     * plan the user just arranged with the priorities it had before it.
     */
    suspend fun setTroikiCategory(
        projectLocalId: Long,
        category: TroikiCategory?,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireProject(projectLocalId)
            if (category != null) {
                if (row.status != ProjectStatus.OPEN) {
                    throw WriteRefused.Placement("only an open project can take a place in the daily plan")
                }
                if (row.troikiCategory != category) rules.assertRoomInTroikiSlot(category)
            }
            db.projects().update(row.copy(troikiCategory = category, updatedAt = at))
            rules.pinnedPriorityOf(projectLocalId)?.let { rules.pinProjectPriority(projectLocalId, it, at) }
            val opId =
                writer.enqueue(
                    SetProjectTroikiOp(projectLocalId, category?.wire),
                    ReplicaEntityKind.PROJECT,
                    projectLocalId,
                )
            QueuedWrite(opId, projectLocalId)
        }

    // --- board columns -------------------------------------------------------

    suspend fun createSection(
        projectLocalId: Long,
        title: String,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            requireProject(projectLocalId)
            val localId =
                db.sections().insert(
                    ProjectSectionRow(
                        projectLocalId = projectLocalId,
                        title = title,
                        position = db.sections().lastPosition(projectLocalId) + 1,
                        createdAt = at,
                        updatedAt = at,
                    ),
                )
            val opId =
                writer.enqueue(
                    CreateSectionOp(localId, projectLocalId, CreateSectionRequest(title)),
                    ReplicaEntityKind.SECTION,
                    localId,
                )
            QueuedWrite(opId, localId)
        }

    suspend fun renameSection(
        sectionLocalId: Long,
        title: String,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireSection(sectionLocalId)
            db.sections().update(row.copy(title = title, updatedAt = at))
            val opId =
                writer.enqueue(
                    PatchSectionOp(sectionLocalId, PatchSectionRequest(title = title)),
                    ReplicaEntityKind.SECTION,
                    sectionLocalId,
                )
            QueuedWrite(opId, sectionLocalId)
        }

    /**
     * Removes a board column. The tasks filed in it stay in the project and become
     * unfiled, which is what the replica's own rule already does and what the
     * server does too.
     */
    suspend fun deleteSection(sectionLocalId: Long): QueuedWrite =
        writer.transaction {
            val row = requireSection(sectionLocalId)
            val opId =
                writer.enqueue(
                    DeleteSectionOp(sectionLocalId, row.serverId),
                    ReplicaEntityKind.SECTION,
                    sectionLocalId,
                )
            db.sections().delete(row)
            QueuedWrite(opId, sectionLocalId)
        }

    /**
     * Moves a column along its board.
     *
     * The whole board is renumbered, not just the column that moved, because
     * that is what the server does with the same request: it reinserts the
     * column and renumbers every column to `0..n-1`. Writing only the moved one
     * would leave two columns claiming the same position until the next pull,
     * and the board would visibly settle a second time under the user's hand.
     * The rule itself lives in [boardAfterMove], which is held to the server's
     * answers by a shared set of cases.
     *
     * [position] is where the user dropped the column. It is clamped to the
     * board rather than refused: a drag past either end means the end.
     */
    suspend fun reorderSection(
        sectionLocalId: Long,
        position: Int,
    ): QueuedWrite =
        writer.transaction {
            val at = writer.now()
            val row = requireSection(sectionLocalId)
            val board = db.sections().forProject(row.projectLocalId)
            val order = boardAfterMove(board.map { it.localId }, sectionLocalId, position)
            val byLocalId = board.associateBy { it.localId }
            order.forEachIndexed { index, localId ->
                val column = byLocalId.getValue(localId)
                if (column.position != index) db.sections().update(column.copy(position = index, updatedAt = at))
            }
            val opId =
                writer.enqueue(
                    ReorderSectionOp(sectionLocalId, position),
                    ReplicaEntityKind.SECTION,
                    sectionLocalId,
                )
            QueuedWrite(opId, sectionLocalId)
        }

    // --- internals -----------------------------------------------------------

    private fun statusAfter(action: ProjectStatusAction): ProjectStatus =
        when (action) {
            ProjectStatusAction.COMPLETE -> ProjectStatus.COMPLETED
            ProjectStatusAction.CANCEL -> ProjectStatus.CANCELLED
            ProjectStatusAction.ARCHIVE -> ProjectStatus.ARCHIVED
            ProjectStatusAction.UNCOMPLETE, ProjectStatusAction.UNARCHIVE -> ProjectStatus.OPEN
        }

    private suspend fun requireProject(projectLocalId: Long): ProjectRow =
        db.projects().byLocalId(projectLocalId)
            ?: throw WriteRefused.RowMissing("project", projectLocalId)

    private suspend fun requireSection(sectionLocalId: Long): ProjectSectionRow =
        db.sections().byLocalId(sectionLocalId)
            ?: throw WriteRefused.RowMissing("board column", sectionLocalId)

    /** Labels are resolved, never invented: a name this device does not know is left to the server. */
    private suspend fun resolveLabels(names: List<String>): List<Long> =
        names.mapNotNull { db.labels().byName(it)?.localId }
}
