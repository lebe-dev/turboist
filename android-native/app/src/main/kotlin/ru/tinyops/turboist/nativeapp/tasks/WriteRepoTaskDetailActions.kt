package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.view.TaskRelationGroup
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The detail screen's writes, made against the shared write path.
 *
 * Nothing is decided here. Every call applies its change to the replica and
 * queues it for the server in one transaction, and refuses whatever the server
 * would refuse; this class only names the writes one task screen makes.
 */
@Singleton
class WriteRepoTaskDetailActions
    @Inject
    constructor(
        private val tasks: TaskWriteRepo,
    ) : TaskDetailActions {
        override suspend fun complete(taskLocalId: Long) {
            tasks.complete(taskLocalId)
        }

        override suspend fun uncomplete(taskLocalId: Long) {
            tasks.uncomplete(taskLocalId)
        }

        override suspend fun edit(
            taskLocalId: Long,
            edit: TaskEdit,
        ) {
            tasks.patch(taskLocalId, edit)
        }

        override suspend fun cancel(taskLocalId: Long) {
            tasks.cancel(taskLocalId)
        }

        override suspend fun pin(taskLocalId: Long) {
            tasks.pin(taskLocalId)
        }

        override suspend fun unpin(taskLocalId: Long) {
            tasks.unpin(taskLocalId)
        }

        override suspend fun duplicate(taskLocalId: Long) {
            tasks.duplicate(taskLocalId)
        }

        override suspend fun decompose(
            taskLocalId: Long,
            titles: List<String>,
        ) {
            tasks.decompose(taskLocalId, titles)
        }

        override suspend fun plan(
            taskLocalId: Long,
            state: PlanState,
        ) {
            tasks.plan(taskLocalId, state)
        }

        override suspend fun move(
            taskLocalId: Long,
            destination: TaskDestination,
        ) {
            tasks.move(taskLocalId, destination)
        }

        override suspend fun delete(taskLocalId: Long) {
            tasks.delete(taskLocalId)
        }

        /**
         * A subtask is created with a title and nothing else: the rest it inherits
         * from its parent, which is what the create endpoint does with a subtask
         * whose label field is absent.
         */
        override suspend fun createSubtask(
            parentTaskLocalId: Long,
            title: String,
        ) {
            tasks.create(TaskDestination.SubtaskOf(parentTaskLocalId), NewTask(title = title))
        }

        /**
         * The direction travels with the link because it is read relative to the
         * task the user is looking at: the same stored edge is "blocked by" from
         * one end and "blocks" from the other.
         */
        override suspend fun addRelation(
            taskLocalId: Long,
            peerTaskLocalId: Long,
            group: TaskRelationGroup,
        ) {
            tasks.addRelation(taskLocalId, peerTaskLocalId, group.type, group.direction)
        }

        override suspend fun removeRelation(
            taskLocalId: Long,
            relationLocalId: Long,
        ) {
            tasks.removeRelation(taskLocalId, relationLocalId)
        }
    }
