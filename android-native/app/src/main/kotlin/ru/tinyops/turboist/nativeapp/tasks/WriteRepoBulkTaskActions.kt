package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import ru.tinyops.turboist.core.sync.write.WriteRefused
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The selection actions, made against the shared write path.
 *
 * Nothing is decided here. Where the API has a bulk endpoint the repository has
 * a matching call that changes every row and queues one request in a single
 * transaction; this class only names which one each action is.
 *
 * The two actions the API has no bulk endpoint for are written out as a loop,
 * and deliberately not disguised as anything else: they really are one request
 * per task, and a fake batch op would be an entry in the queue's vocabulary that
 * no endpoint could ever answer.
 */
@Singleton
class WriteRepoBulkTaskActions
    @Inject
    constructor(
        private val tasks: TaskWriteRepo,
    ) : BulkTaskActions {
        override suspend fun complete(taskLocalIds: List<Long>): BulkChange {
            val written = tasks.bulkComplete(taskLocalIds)
            return BulkChange(changed = written.accepted.size, leftBlocked = written.refused.size)
        }

        override suspend fun move(
            taskLocalIds: List<Long>,
            destination: TaskDestination,
        ): BulkChange = BulkChange(tasks.bulkMove(taskLocalIds, destination).accepted.size)

        override suspend fun prioritise(
            taskLocalIds: List<Long>,
            priority: Priority,
        ): BulkChange {
            val written = tasks.bulkPriority(taskLocalIds, priority)
            return BulkChange(changed = written.accepted.size, leftLocked = written.locked.size)
        }

        override suspend fun plan(
            taskLocalIds: List<Long>,
            state: PlanState,
        ): BulkChange = BulkChange(eachRemaining(taskLocalIds) { tasks.plan(it, state) })

        /**
         * Deletes the selection, one task at a time.
         *
         * A task takes its subtasks with it, so a selection holding both a task
         * and something under it runs out of rows partway through. That is the
         * right outcome rather than an error: the subtask is gone because the
         * user deleted the thing it was part of.
         */
        override suspend fun delete(taskLocalIds: List<Long>): BulkChange =
            BulkChange(eachRemaining(taskLocalIds) { tasks.delete(it) })

        override suspend fun group(
            title: String,
            childTaskLocalIds: List<Long>,
            destination: TaskDestination,
        ): BulkChange {
            tasks.group(
                title = title,
                childTaskLocalIds = childTaskLocalIds,
                destination = destination,
            )
            return BulkChange(childTaskLocalIds.distinct().size)
        }

        /**
         * Runs [write] over every task still in the replica, and counts those.
         *
         * A row that has already gone is skipped rather than failing the rest:
         * the only way it can happen is that an earlier task in the same
         * selection took it along, and the user is not owed an error for that.
         */
        private suspend fun eachRemaining(
            taskLocalIds: List<Long>,
            write: suspend (Long) -> Unit,
        ): Int {
            var changed = 0
            for (taskLocalId in taskLocalIds.distinct()) {
                try {
                    write(taskLocalId)
                    changed++
                } catch (missing: WriteRefused.RowMissing) {
                    continue
                }
            }
            return changed
        }
    }
