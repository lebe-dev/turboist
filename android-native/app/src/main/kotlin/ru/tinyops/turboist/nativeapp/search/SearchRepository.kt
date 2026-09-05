package ru.tinyops.turboist.nativeapp.search

import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.SearchDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the search screen asks.
 *
 * A narrow port, so the screen depends on "answer this query" rather than on the
 * replica, and so the screen can be exercised without a database behind it.
 */
interface SearchRepository {
    /**
     * Answers [typed] over the whole device.
     *
     * A query too short to be worth running comes back empty rather than
     * refused — deciding whether to ask is the caller's business, and this must
     * be safe to call on every keystroke.
     */
    suspend fun search(
        typed: String,
        filters: SearchFilters,
    ): SearchResults
}

/**
 * Search answered from the on-device replica, and from nothing else.
 *
 * No request is made and none is needed: every record the user can see is
 * already here, so search is one of the few things that works better without a
 * connection than with one. There is no loading state to speak of and no page to
 * fetch — only a query, and the rows it names.
 */
@Singleton
class ReplicaSearchRepository
    @Inject
    constructor(
        private val search: SearchDao,
        private val projects: ProjectDao,
    ) : SearchRepository {
        override suspend fun search(
            typed: String,
            filters: SearchFilters,
        ): SearchResults {
            val query = FtsQuery.match(typed) ?: return SearchResults()
            val titleQuery = FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed) ?: return SearchResults()

            val taskRows =
                if (!filters.includes(SearchKind.TASKS)) {
                    emptyList()
                } else {
                    search.tasks(
                        query = query,
                        titleQuery = titleQuery,
                        status = if (filters.openTasksOnly) TaskStatus.OPEN.wire else null,
                        limit = PER_KIND_LIMIT,
                    )
                }

            return SearchResults(
                tasks = withProjectTitles(taskRows.map { it.toDomain() }),
                projects =
                    if (!filters.includes(SearchKind.PROJECTS)) {
                        emptyList()
                    } else {
                        search.projects(query, titleQuery, PER_KIND_LIMIT).map { it.toDomain() }
                    },
                labels =
                    if (!filters.includes(SearchKind.LABELS)) {
                        emptyList()
                    } else {
                        search.labels(query, PER_KIND_LIMIT).map { it.toDomain() }
                    },
                contexts =
                    if (!filters.includes(SearchKind.CONTEXTS)) {
                        emptyList()
                    } else {
                        search.contexts(query, PER_KIND_LIMIT).map { it.toDomain() }
                    },
            )
        }

        /**
         * Names the project each task sits in.
         *
         * Looked up once per distinct project rather than once per row: a page of
         * results usually comes from a handful of projects, and the whole list of
         * projects is not worth reading to answer a question about five of them.
         */
        private suspend fun withProjectTitles(tasks: List<Task>): List<TaskHit> {
            if (tasks.isEmpty()) return emptyList()
            val titles =
                tasks
                    .mapNotNull { it.projectLocalId }
                    .distinct()
                    .mapNotNull { localId -> projects.byLocalId(localId)?.let { localId to it.title } }
                    .toMap()
            return tasks.map { TaskHit(it, it.projectLocalId?.let(titles::get)) }
        }

        private companion object {
            /**
             * How many rows one kind can contribute to an answer.
             *
             * Search is a way to reach one thing, not a way to page the
             * workspace: past a screenful or two the answer stops being useful
             * and the query starts costing what the screen was meant to save.
             * The cap is applied after the ranking, so what it drops is always
             * the worst matches.
             */
            const val PER_KIND_LIMIT = 50
        }
    }
