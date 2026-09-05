package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.ContextDao
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.ProjectSectionDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.nativeapp.tasks.TaskListRepository
import javax.inject.Inject
import javax.inject.Singleton

/**
 * One context and the projects filed under it, as the browsing screen draws it.
 *
 * The group carries its context rather than only its name because tapping the
 * heading opens that context, and a heading that only knew a name could not lead
 * anywhere.
 */
data class ProjectGroup(
    val context: Context,
    val projects: List<Project>,
)

/** Everything the screen showing one project board reads. */
data class ProjectBoardContent(
    val project: Project,
    val contextName: String?,
    val sections: List<ProjectSection>,
    val tasks: List<Task>,
)

/** Everything the screen showing one context reads. */
data class ContextContent(
    val context: Context,
    val projects: List<Project>,
    val tasks: List<Task>,
)

/**
 * What the project and context screens read.
 *
 * Like every other list in the app these are standing queries over the replica,
 * never fetches: a project renamed here, a column added on another device and a
 * board reordered while the phone was in a tunnel all reach the screen the same
 * way — the rows behind it changed. There is no loading state to speak of and
 * nothing here can fail for want of a connection.
 */
@Singleton
class ProjectsRepository
    @Inject
    constructor(
        private val contexts: ContextDao,
        private val projects: ProjectDao,
        private val sections: ProjectSectionDao,
        private val tasks: TaskListRepository,
    ) {
        /**
         * Every project, under the context it belongs to.
         *
         * The contexts lead, in their own order, and each carries its projects;
         * a context with none still appears, because it is where the user files
         * the next one. A project whose context is gone is left out — deleting a
         * context takes its projects with it on both sides, so such a row can
         * only be a half-applied change that the next pull completes.
         */
        fun observeGroups(): Flow<List<ProjectGroup>> =
            combine(contexts.observeAll(), projects.observeAll()) { contextRows, projectRows ->
                val byContext = projectRows.groupBy { it.contextLocalId }
                contextRows.map { context ->
                    ProjectGroup(
                        context = context.toDomain(),
                        projects = byContext[context.localId].orEmpty().map { it.toDomain() },
                    )
                }
            }

        /** The contexts a new project can be filed under. */
        fun observeContexts(): Flow<List<Context>> = contexts.observeAll().map { rows -> rows.map { it.toDomain() } }

        /** One project with its board and its work, or `null` once the project is gone. */
        fun observeBoard(projectLocalId: Long): Flow<ProjectBoardContent?> =
            combine(
                projects.observeByLocalId(projectLocalId),
                sections.observeForProject(projectLocalId),
                tasks.observeProject(projectLocalId),
                contexts.observeAll(),
            ) { projectRow, sectionRows, taskRows, contextRows ->
                val project = projectRow?.toDomain() ?: return@combine null
                ProjectBoardContent(
                    project = project,
                    contextName = contextRows.firstOrNull { it.localId == project.contextLocalId }?.name,
                    sections = sectionRows.map { it.toDomain() },
                    tasks = taskRows,
                )
            }

        /** One context with its projects and its work, or `null` once the context is gone. */
        fun observeContext(contextLocalId: Long): Flow<ContextContent?> =
            combine(
                contexts.observeByLocalId(contextLocalId),
                projects.observeAll(),
                tasks.observeContext(contextLocalId),
            ) { contextRow, projectRows, taskRows ->
                val context = contextRow?.toDomain() ?: return@combine null
                ContextContent(
                    context = context,
                    projects = projectRows.filter { it.contextLocalId == contextLocalId }.map { it.toDomain() },
                    tasks = taskRows,
                )
            }
    }
