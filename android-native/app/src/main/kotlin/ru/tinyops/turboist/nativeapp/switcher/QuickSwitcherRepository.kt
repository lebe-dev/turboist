package ru.tinyops.turboist.nativeapp.switcher

import ru.tinyops.turboist.core.database.dao.SearchDao
import ru.tinyops.turboist.core.database.dao.SwitcherHit
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.Task
import javax.inject.Inject
import javax.inject.Singleton

/** Everything one quick-switcher query matched, each kind in rank order. */
data class SwitcherMatches(
    val tasks: List<RankedRow> = emptyList(),
    val projects: List<RankedRow> = emptyList(),
)

/**
 * What the quick switcher asks of the workspace.
 *
 * A narrow port, so the presenter can be checked without a database.
 */
interface QuickSwitcherRepository {
    /** Tasks and projects matching [typed]; empty when the query is too short to run. */
    suspend fun search(typed: String): SwitcherMatches

    /** The rows [refs] name, in the same order; a ref whose row is gone is dropped. */
    suspend fun resolve(refs: List<RecentRef>): List<SwitcherRow>

    /** The project a task sits in, or `null` for the inbox or a task this device no longer holds. */
    suspend fun projectOfTask(taskLocalId: Long): Long? = null
}

/**
 * The quick switcher answered from the on-device replica alone.
 *
 * No request is made, so it works the same with no connection at all — which on
 * a phone is often exactly when someone needs to find a task.
 */
@Singleton
class ReplicaQuickSwitcherRepository
    @Inject
    constructor(
        private val search: SearchDao,
    ) : QuickSwitcherRepository {
        override suspend fun search(typed: String): SwitcherMatches {
            val query = FtsQuery.match(typed) ?: return SwitcherMatches()
            val titleQuery = FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed) ?: return SwitcherMatches()
            val taskHits = search.switcherTasks(query, titleQuery, query, PER_KIND_LIMIT)
            val projectHits = search.switcherProjects(query, titleQuery, query, PER_KIND_LIMIT)
            val tasks = tasksById(taskHits.map { it.localId })
            val projects = projectsById(projectHits.map { it.localId })
            val projectTitles = projectTitles(tasks.values)
            return SwitcherMatches(
                tasks = taskHits.ranked(tasks) { SwitcherRow.TaskRow(it, it.projectLocalId?.let(projectTitles::get)) },
                projects = projectHits.ranked(projects) { SwitcherRow.ProjectRow(it) },
            )
        }

        override suspend fun resolve(refs: List<RecentRef>): List<SwitcherRow> {
            if (refs.isEmpty()) return emptyList()
            val tasks = tasksById(refs.filter { it.kind == SwitcherKind.TASK }.map { it.localId })
            val projects = projectsById(refs.filter { it.kind == SwitcherKind.PROJECT }.map { it.localId })
            val projectTitles = projectTitles(tasks.values)
            return refs.mapNotNull { ref ->
                when (ref.kind) {
                    SwitcherKind.TASK ->
                        tasks[ref.localId]?.let { SwitcherRow.TaskRow(it, it.projectLocalId?.let(projectTitles::get)) }
                    SwitcherKind.PROJECT -> projects[ref.localId]?.let { SwitcherRow.ProjectRow(it) }
                }
            }
        }

        override suspend fun projectOfTask(taskLocalId: Long): Long? =
            tasksById(listOf(taskLocalId))[taskLocalId]?.projectLocalId

        private suspend fun tasksById(ids: List<Long>): Map<Long, Task> =
            if (ids.isEmpty()) {
                emptyMap()
            } else {
                search.tasksByLocalIds(
                    ids,
                ).map { it.toDomain() }.associateBy { it.localId }
            }

        private suspend fun projectsById(ids: List<Long>): Map<Long, Project> =
            if (ids.isEmpty()) {
                emptyMap()
            } else {
                search.projectsByLocalIds(ids).map { it.toDomain() }.associateBy { it.localId }
            }

        /** One lookup for every project the tasks sit in, rather than one per row. */
        private suspend fun projectTitles(tasks: Collection<Task>): Map<Long, String> =
            projectsById(tasks.mapNotNull { it.projectLocalId }.distinct()).mapValues { it.value.title }

        private fun <T> List<SwitcherHit>.ranked(
            rows: Map<Long, T>,
            build: (T) -> SwitcherRow,
        ): List<RankedRow> = mapNotNull { hit -> rows[hit.localId]?.let { RankedRow(build(it), hit.hitRank) } }

        private companion object {
            /** The cap is applied after ranking, so what it drops is always the worst matches. */
            const val PER_KIND_LIMIT = 30
        }
    }
