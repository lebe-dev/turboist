package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.sync.EngineSyncScheduler
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.ApiOlderCompletions
import ru.tinyops.turboist.nativeapp.tasks.BulkTaskActions
import ru.tinyops.turboist.nativeapp.tasks.CompletedHistoryActions
import ru.tinyops.turboist.nativeapp.tasks.CompletedHistorySource
import ru.tinyops.turboist.nativeapp.tasks.OlderCompletions
import ru.tinyops.turboist.nativeapp.tasks.ReplicaCompletedHistory
import ru.tinyops.turboist.nativeapp.tasks.ReplicaTaskRelationSearch
import ru.tinyops.turboist.nativeapp.tasks.TaskDetailActions
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import ru.tinyops.turboist.nativeapp.tasks.TaskRelationSearch
import ru.tinyops.turboist.nativeapp.tasks.WriteRepoBulkTaskActions
import ru.tinyops.turboist.nativeapp.tasks.WriteRepoCompletedHistoryActions
import ru.tinyops.turboist.nativeapp.tasks.WriteRepoTaskDetailActions
import ru.tinyops.turboist.nativeapp.tasks.WriteRepoTaskListActions
import java.time.Clock
import javax.inject.Singleton

/**
 * The seams the task list screens depend on.
 *
 * The screens ask for interfaces so that what a row does when it is ticked, and
 * what "now" means, can both be replaced in a test without a screen changing.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class TasksModule {
    @Binds
    @Singleton
    abstract fun bindTaskListActions(actions: WriteRepoTaskListActions): TaskListActions

    /**
     * What a picked set of tasks can be put through. A port of its own beside the
     * single-task one, because a selection is changed by different calls — three
     * of which the API answers for the whole selection in one request.
     */
    @Binds
    @Singleton
    abstract fun bindBulkTaskActions(actions: WriteRepoBulkTaskActions): BulkTaskActions

    /**
     * Everything one task screen can do to the task it is showing. A wider port
     * than a list's, and a separate binding, so a list screen still depends only
     * on the four writes it makes.
     */
    @Binds
    @Singleton
    abstract fun bindTaskDetailActions(actions: WriteRepoTaskDetailActions): TaskDetailActions

    /**
     * The completion history's two sources. The device's own copy answers the
     * recent stretch with no connection; the server answers for anything older,
     * and only when somebody scrolls that far.
     */
    @Binds
    @Singleton
    abstract fun bindCompletedHistorySource(source: ReplicaCompletedHistory): CompletedHistorySource

    @Binds
    @Singleton
    abstract fun bindOlderCompletions(completions: ApiOlderCompletions): OlderCompletions

    /**
     * Reopening a finished task, including one the device has no copy of — that
     * one is taken into the replica first, so the queue has a row to name.
     */
    @Binds
    @Singleton
    abstract fun bindCompletedHistoryActions(actions: WriteRepoCompletedHistoryActions): CompletedHistoryActions

    /**
     * Where the relation picker looks for the other end of a link: the device's
     * own index, never the server, so linking works with no connection.
     */
    @Binds
    @Singleton
    abstract fun bindTaskRelationSearch(search: ReplicaTaskRelationSearch): TaskRelationSearch

    /**
     * A pull-to-refresh runs a real sync cycle — the same one every automatic
     * trigger runs, so a screen refreshed by hand and one refreshed by the app
     * itself are refreshed by the same code.
     */
    @Binds
    @Singleton
    abstract fun bindSyncScheduler(scheduler: EngineSyncScheduler): SyncScheduler

    companion object {
        /**
         * The wall clock, in the device's own zone.
         *
         * The zone matters as much as the instant: a day view is a question
         * about the user's calendar day, so it has to be measured where the user
         * is rather than in UTC.
         */
        @Provides
        @Singleton
        fun clock(): Clock = Clock.systemDefaultZone()
    }
}
