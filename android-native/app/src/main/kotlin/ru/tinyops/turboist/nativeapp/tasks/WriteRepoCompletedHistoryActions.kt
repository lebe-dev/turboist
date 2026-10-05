package ru.tinyops.turboist.nativeapp.tasks

import androidx.room.withTransaction
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.toRow
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Reopening a finished task, whichever of the history's two sources it came from.
 *
 * A task the device already holds is reopened like any other. A task read from
 * the server for this screen has no row here to change, and the queue carries
 * device ids rather than server ones — so the row is taken into the replica first
 * and reopened straight afterwards, both in one transaction. Nothing is invented
 * by doing so: the row is written from the server's own answer, and the catch-up
 * that follows the queued write replaces it with the server's current version,
 * placement and all.
 *
 * Taking it in is also what makes it stay. The device keeps a bounded stretch of
 * *finished* work; an open task is not history, so the row that comes back is
 * live work the device holds like any other.
 */
@Singleton
class WriteRepoCompletedHistoryActions
    @Inject
    constructor(
        private val db: TurboistDatabase,
        private val tasks: TaskWriteRepo,
    ) : CompletedHistoryActions {
        override suspend fun uncomplete(task: Task) {
            if (task.localId != NO_LOCAL_ID) {
                tasks.uncomplete(task.localId)
                return
            }
            val serverId =
                requireNotNull(task.serverId) {
                    "a task with neither a device id nor a server id cannot be reopened"
                }
            db.withTransaction {
                val localId =
                    db.tasks().localIdForServerId(serverId)
                        ?: db.tasks().upsertByServerId(task.copy(localId = NO_LOCAL_ID).toRow())
                tasks.uncomplete(localId)
            }
        }
    }
