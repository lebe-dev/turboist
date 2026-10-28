package ru.tinyops.turboist.nativeapp.shell

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOn
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.TaskViewDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.Task
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The projects and tasks the user pinned, as the drawer lists them.
 *
 * It is the web sidebar's "Pinned" section: a shelf of shortcuts that sits
 * beside the destinations, so the things the user keeps in front of them are one
 * tap away from anywhere. Only open work is on it — a finished project or a
 * completed task has nothing left to be reached for.
 */
data class DrawerPinned(
    val projects: List<Project> = emptyList(),
    val tasks: List<Task> = emptyList(),
) {
    val isEmpty: Boolean get() = projects.isEmpty() && tasks.isEmpty()
}

/** Where the drawer's pinned shelf comes from. */
interface DrawerPinnedSource {
    val pinned: Flow<DrawerPinned>
}

/**
 * The pinned shelf read off the replica.
 *
 * Both halves are standing queries, so pinning from a project or task screen —
 * here or on another device — puts the entry on the shelf without the drawer
 * asking. Projects come oldest pin first, the order the projects screen uses;
 * tasks in the shared list order.
 */
@Singleton
class ReplicaDrawerPinnedSource
    @Inject
    constructor(
        private val projects: ProjectDao,
        private val views: TaskViewDao,
    ) : DrawerPinnedSource {
        override val pinned: Flow<DrawerPinned> =
            combine(projects.observeAll(), views.queryPinned(null)) { projectRows, taskRows ->
                DrawerPinned(
                    projects =
                        projectRows
                            .asSequence()
                            .filter { it.isPinned }
                            .map { it.toDomain() }
                            .filter { it.status == ProjectStatus.OPEN }
                            .sortedBy { it.pinnedAt ?: Long.MAX_VALUE }
                            .toList(),
                    tasks = taskRows.map { it.toDomain() },
                )
            }.flowOn(Dispatchers.Default)
    }
