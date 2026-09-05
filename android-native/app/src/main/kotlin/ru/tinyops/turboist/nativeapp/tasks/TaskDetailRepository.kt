package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.flowOn
import ru.tinyops.turboist.core.database.dao.ContextDao
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.OpenBlocker
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.ProjectSectionDao
import ru.tinyops.turboist.core.database.dao.TaskDao
import ru.tinyops.turboist.core.database.dao.TaskHydrationDao
import ru.tinyops.turboist.core.database.dao.TaskLabelJoin
import ru.tinyops.turboist.core.database.dao.TaskParentLink
import ru.tinyops.turboist.core.database.dao.TaskRelationDao
import ru.tinyops.turboist.core.database.dao.TaskRelationEdge
import ru.tinyops.turboist.core.database.dao.TaskRelationPeer
import ru.tinyops.turboist.core.database.dao.TaskViewDao
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.view.BlockEdge
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.core.model.view.openBlockerLocalIds
import ru.tinyops.turboist.core.model.view.taskRelationSummaries
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the task detail screen reads.
 *
 * Like every list, it is a standing query over the replica rather than a fetch:
 * the screen shows what the device holds, an edit made here changes it under the
 * same query, and a change that arrived from another device reaches the screen
 * by exactly the same route. There is deliberately no network path in this
 * class — nothing it answers depends on the server being reachable.
 */
@Singleton
class TaskDetailRepository
    @Inject
    constructor(
        private val tasks: TaskDao,
        private val views: TaskViewDao,
        private val hydration: TaskHydrationDao,
        private val taskRelations: TaskRelationDao,
        private val projects: ProjectDao,
        private val sections: ProjectSectionDao,
        private val contexts: ContextDao,
        private val labels: LabelDao,
    ) {
        /**
         * Turns an address into the device id of the task it names.
         *
         * A local address is already the answer. A server id has to be looked up,
         * and may legitimately find nothing: a link can name a task this device
         * has not replicated yet, or one that was deleted elsewhere. The lookup
         * repeats whenever the task table changes, so a link opened a moment
         * before the row arrives resolves itself instead of staying dead.
         */
        fun observeLocalId(address: TaskAddress): Flow<Long?> =
            when (address) {
                is TaskAddress.Local -> flowOf(address.taskLocalId)
                is TaskAddress.Server -> tasks.observeLocalIdForServerId(address.serverId).distinctUntilChanged()
            }

        /**
         * Everything the screen shows about one task, or `null` once the task is
         * gone.
         *
         * Assembled away from the main thread. Half of what it reads answers for
         * the whole workspace — every tagging, every relation edge, every parent
         * link — and all of it is read again on every write the replica takes,
         * a syncing page included. That is far too much to do on the thread that
         * draws, and a detail screen is open exactly while a sync is most likely
         * to be writing.
         */
        fun observeDetail(taskLocalId: Long): Flow<TaskDetailContent?> =
            combine(
                tasks.observeByLocalId(taskLocalId),
                views.querySubtree(taskLocalId),
                hydration.observeTaskLabels(),
                relationFacts(taskLocalId),
                workspace(),
            ) { row, subtree, labelJoins, facts, workspace ->
                row ?: return@combine null
                assemble(row, subtree, labelJoins, facts, workspace)
            }.flowOn(Dispatchers.Default)

        /** The places a task can be moved to: the inbox, every project, every column. */
        fun observeMoveOptions(): Flow<List<MoveProject>> =
            combine(projects.observeAll(), sections.observeAll()) { projectRows, sectionRows ->
                val columns = sectionRows.groupBy { it.projectLocalId }
                projectRows.map { project ->
                    MoveProject(
                        projectLocalId = project.localId,
                        title = project.title,
                        sections =
                            columns[project.localId].orEmpty().map {
                                MoveSection(it.localId, it.title)
                            },
                    )
                }
            }

        /**
         * Every relation fact the screen needs, read together so they always
         * describe one instant.
         *
         * Three of them are about the workspace — the edges, the parent links and
         * the open blockers the padlock rule works over — and the fourth is this
         * task's own links with their peers named. Read apart they could disagree
         * for a frame: a link listed as still waiting on work that the same frame
         * shows as finished.
         */
        private fun relationFacts(taskLocalId: Long): Flow<RelationFacts> =
            combine(
                hydration.observeRelationEdges(),
                hydration.observeParentLinks(),
                hydration.observeOpenBlockers(),
                taskRelations.observePeersForTask(taskLocalId),
            ) { edges, parents, blockers, peers -> RelationFacts(edges, parents, blockers, peers) }

        /** The named things a placement resolves against, and the labels a picker offers. */
        private fun workspace(): Flow<Workspace> =
            combine(
                projects.observeAll(),
                sections.observeAll(),
                contexts.observeAll(),
                labels.observeAll(),
            ) { projectRows, sectionRows, contextRows, labelRows ->
                Workspace(projectRows, sectionRows, contextRows, labelRows)
            }

        private suspend fun assemble(
            row: TaskRow,
            subtree: List<TaskRow>,
            labelJoins: List<TaskLabelJoin>,
            relations: RelationFacts,
            workspace: Workspace,
        ): TaskDetailContent {
            val labelsByTask = labelJoins.groupBy({ it.taskLocalId }, { it.label.toDomain() })
            val localIds = listOf(row.localId) + subtree.map { it.localId }
            val parentOf = relations.parents.associate { it.taskLocalId to it.parentLocalId }
            val summaries =
                taskRelationSummaries(
                    taskLocalIds = localIds,
                    openBlockEdges = relations.openBlockEdges(),
                    relationCounts = relations.counts(),
                    parentOf = parentOf,
                )

            fun hydrate(task: TaskRow): Task =
                task.toDomain().copy(
                    labels = labelsByTask[task.localId].orEmpty(),
                    relationSummary = summaries[task.localId] ?: TaskRelationSummary.NONE,
                )
            return TaskDetailContent(
                task = hydrate(row),
                placement = placementOf(row, workspace),
                blockers = blockersOf(row.localId, relations, parentOf),
                relations = relations.peers.mapNotNull(::linkOf),
                subtasks = subtree.map(::hydrate),
                projectTitles = workspace.projects.associate { it.localId to it.title },
                knownLabels = workspace.labels.map(LabelRow::toDomain),
                // A project standing in the daily plan fixes the priority of its
                // work, so the field belongs to the project rather than to the
                // task and the screen has to stop offering it.
                priorityLockedByTroiki =
                    row.projectLocalId?.let { id ->
                        workspace.projects.firstOrNull { it.localId == id }?.troikiCategory != null
                    } ?: false,
            )
        }

        /**
         * One stored edge as the screen reads it.
         *
         * An edge whose kind this build does not know is left out rather than
         * filed under one of the three headings. A newer server may name a kind
         * that did not exist when this app was written, and showing it as the
         * wrong thing is worse than not showing it: the user would act on it.
         */
        private fun linkOf(peer: TaskRelationPeer): TaskRelationRef? {
            val direction = if (peer.outgoing) RelationDirection.OUTGOING else RelationDirection.INCOMING
            val group = TaskRelationGroup.of(peer.type, direction) ?: return null
            return TaskRelationRef(
                relationLocalId = peer.relationLocalId,
                group = group,
                peerLocalId = peer.peerLocalId,
                peerServerId = peer.peerServerId,
                peerTitle = peer.peerTitle,
                peerStatus = peer.peerStatus,
            )
        }

        /** The open work standing in the way, named and in the order the rule finds it. */
        private fun blockersOf(
            taskLocalId: Long,
            relations: RelationFacts,
            parentOf: Map<Long, Long>,
        ): List<BlockerRef> {
            val ids = openBlockerLocalIds(taskLocalId, relations.openBlockEdges(), parentOf)
            if (ids.isEmpty()) return emptyList()
            val named = relations.blockers.associateBy { it.blockerLocalId }
            return ids.mapNotNull { id ->
                named[id]?.let { BlockerRef(it.blockerLocalId, it.blockerServerId, it.blockerTitle) }
            }
        }

        private suspend fun placementOf(
            row: TaskRow,
            workspace: Workspace,
        ): TaskPlacement {
            val project = row.projectLocalId?.let { id -> workspace.projects.firstOrNull { it.localId == id } }
            val section = row.sectionLocalId?.let { id -> workspace.sections.firstOrNull { it.localId == id } }
            // A section names its project even when the task does not, which is
            // what keeps a task filed in a column from looking project-less.
            val owningProject =
                project ?: section?.let { column ->
                    workspace.projects.firstOrNull { it.localId == column.projectLocalId }
                }
            val context = row.contextLocalId?.let { id -> workspace.contexts.firstOrNull { it.localId == id } }
            return TaskPlacement(
                inInbox = row.inboxId != null,
                contextLocalId = context?.localId,
                contextName = context?.name,
                projectLocalId = owningProject?.localId,
                projectTitle = owningProject?.title,
                sectionLocalId = section?.localId,
                sectionTitle = section?.title,
                parentLocalId = row.parentLocalId,
                parentTitle = row.parentLocalId?.let { tasks.byLocalId(it)?.title },
            )
        }

        /** Every relation fact the screen needs, as one value. */
        private data class RelationFacts(
            val edges: List<TaskRelationEdge>,
            val parents: List<TaskParentLink>,
            val blockers: List<OpenBlocker>,
            val peers: List<TaskRelationPeer>,
        ) {
            fun openBlockEdges(): List<BlockEdge> = openBlockEdgesOf(edges)

            fun counts(): Map<Long, Int> = relationCountsOf(edges)
        }

        /** The named rows a placement and a label picker resolve against. */
        private data class Workspace(
            val projects: List<ProjectRow>,
            val sections: List<ProjectSectionRow>,
            val contexts: List<ContextRow>,
            val labels: List<LabelRow>,
        )
    }
