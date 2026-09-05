package ru.tinyops.turboist.nativeapp.quickadd

import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Capture's single write, made against the shared write path.
 *
 * Nothing is decided here. The write path inserts the task and queues the
 * request in one transaction, attaches whatever labels the installation's rules
 * call for, and refuses what the server would refuse. This class only names the
 * one call capture makes.
 */
@Singleton
class WriteRepoQuickAddActions
    @Inject
    constructor(
        private val tasks: TaskWriteRepo,
    ) : QuickAddActions {
        override suspend fun create(
            destination: TaskDestination,
            task: NewTask,
        ) {
            tasks.create(destination, task)
        }
    }
