package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The list screens' four writes, made against the shared write path.
 *
 * Nothing is decided here: every one of these applies its change to the replica
 * and queues it for the server in a single transaction, and refuses what the
 * server would refuse. This class only names the four the lists use.
 */
@Singleton
class WriteRepoTaskListActions
    @Inject
    constructor(
        private val tasks: TaskWriteRepo,
    ) : TaskListActions {
        override suspend fun complete(taskLocalId: Long) {
            tasks.complete(taskLocalId)
        }

        override suspend fun uncomplete(taskLocalId: Long) {
            tasks.uncomplete(taskLocalId)
        }

        override suspend fun park(taskLocalId: Long) {
            tasks.plan(taskLocalId, PlanState.BACKLOG)
        }

        override suspend fun planForWeek(taskLocalId: Long) {
            tasks.plan(taskLocalId, PlanState.WEEK)
        }
    }
