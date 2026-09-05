package ru.tinyops.turboist.nativeapp.troiki

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.nativeapp.tasks.TaskListRepository
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the daily plan reads.
 *
 * Like every other list in the app this is a standing query over the copied
 * data, never a fetch: a project put into a bucket here, a project taken out of
 * one on another device, and a task ticked off with no network all reach the
 * screen the same way — the rows behind it changed. There is nothing to load and
 * nothing here that can fail for want of a connection.
 *
 * What the server keeps and this cannot see is the room each bucket has *earned*
 * by work finished in the bucket above it. That is a counter, not a record, and
 * it is not part of the copied data; the plan therefore draws the room a bucket
 * starts with and leaves the rest to the server, which has the number.
 */
@Singleton
class TroikiRepository
    @Inject
    constructor(
        private val projects: ProjectDao,
        private val tasks: TaskListRepository,
    ) {
        /**
         * The three buckets, their projects and the work under them.
         *
         * The work is asked for only once the projects are known, and asked for
         * again when that set changes — which is what keeps a project moved
         * between buckets from being drawn with somebody else's tasks for the
         * moment in between.
         */
        @OptIn(ExperimentalCoroutinesApi::class)
        fun observeBoard(): Flow<List<TroikiSlot>> =
            standingProjects()
                .flatMapLatest { standing ->
                    tasks
                        .observeTroikiBoard(standing.map { it.localId })
                        .map { work -> troikiSlots(standing, work) }
                }

        /**
         * The projects a bucket can still be filled with: open work that is not
         * already standing somewhere in the plan.
         */
        fun observeAssignable(): Flow<List<Project>> =
            projects.observeAll().map { rows ->
                rows
                    .map { it.toDomain() }
                    .filter { it.status == ProjectStatus.OPEN && it.troikiCategory == null }
            }

        private fun standingProjects(): Flow<List<Project>> =
            projects
                .observeAll()
                .map { rows ->
                    rows
                        .map { it.toDomain() }
                        .filter { it.status == ProjectStatus.OPEN && it.troikiCategory != null }
                }.distinctUntilChanged()
    }
