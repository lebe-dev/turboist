package ru.tinyops.turboist.nativeapp.troiki

import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.ProjectWriteRepo
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import ru.tinyops.turboist.core.sync.write.TroikiWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The writes the daily plan makes.
 *
 * A port onto the shared write path rather than a second copy of it: each of
 * these applies its change to the copied data and queues the request for the
 * server in one transaction, so a plan rearranged in a tunnel is right on screen
 * at once and right on the server whenever the phone comes back.
 *
 * The writes the plan shares with every list — ticking a task off, parking it,
 * planning it for the week — are not restated here. The plan takes the list port
 * for those, so "complete this task" has one meaning in the app rather than two.
 */
interface TroikiActions {
    /** Puts a project into a bucket, or takes it out of all of them. */
    suspend fun setCategory(
        projectLocalId: Long,
        category: TroikiCategory?,
    )

    /** Begins a cycle, which is what fixes how much room the lower buckets have. */
    suspend fun start()

    /** Ends a cycle and empties every bucket. */
    suspend fun reset()

    /** Writes down a task in one of the plan's projects. */
    suspend fun addTask(
        projectLocalId: Long,
        title: String,
    )
}

/**
 * The plan's writes, made against the shared write path.
 *
 * Nothing is decided here. The write path refuses a bucket that is already full,
 * empties the buckets when a cycle ends, and queues each request beside the
 * change it made. This class only names the calls the screen makes.
 */
@Singleton
class WriteRepoTroikiActions
    @Inject
    constructor(
        private val troiki: TroikiWriteRepo,
        private val projects: ProjectWriteRepo,
        private val tasks: TaskWriteRepo,
    ) : TroikiActions {
        override suspend fun setCategory(
            projectLocalId: Long,
            category: TroikiCategory?,
        ) {
            projects.setTroikiCategory(projectLocalId, category)
        }

        override suspend fun start() {
            troiki.start()
        }

        override suspend fun reset() {
            troiki.reset()
        }

        override suspend fun addTask(
            projectLocalId: Long,
            title: String,
        ) {
            tasks.create(TaskDestination.InProject(projectLocalId), NewTask(title = title))
        }
    }
