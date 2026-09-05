package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.ContextWriteRepo
import ru.tinyops.turboist.core.sync.write.NewProject
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.ProjectEdit
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.ProjectWriteRepo
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The project screens' writes, made against the shared write path.
 *
 * Nothing is decided here. The write path applies each change to the replica and
 * queues the request in one transaction, refuses what the server would refuse,
 * and renumbers a board the way the server renumbers it. This class only names
 * the calls the screens make.
 */
@Singleton
class WriteRepoProjectActions
    @Inject
    constructor(
        private val projects: ProjectWriteRepo,
        private val tasks: TaskWriteRepo,
    ) : ProjectActions {
        override suspend fun createProject(
            contextLocalId: Long,
            project: NewProject,
        ) {
            projects.create(contextLocalId, project)
        }

        override suspend fun editProject(
            projectLocalId: Long,
            edit: ProjectEdit,
        ) {
            projects.patch(projectLocalId, edit)
        }

        override suspend fun setProjectStatus(
            projectLocalId: Long,
            action: ProjectStatusAction,
        ) {
            projects.setStatus(projectLocalId, action)
        }

        override suspend fun pinProject(projectLocalId: Long) {
            projects.pin(projectLocalId)
        }

        override suspend fun unpinProject(projectLocalId: Long) {
            projects.unpin(projectLocalId)
        }

        override suspend fun setTroikiCategory(
            projectLocalId: Long,
            category: TroikiCategory?,
        ) {
            projects.setTroikiCategory(projectLocalId, category)
        }

        override suspend fun deleteProject(projectLocalId: Long) {
            projects.delete(projectLocalId)
        }

        override suspend fun createSection(
            projectLocalId: Long,
            title: String,
        ) {
            projects.createSection(projectLocalId, title)
        }

        override suspend fun renameSection(
            sectionLocalId: Long,
            title: String,
        ) {
            projects.renameSection(sectionLocalId, title)
        }

        override suspend fun deleteSection(sectionLocalId: Long) {
            projects.deleteSection(sectionLocalId)
        }

        override suspend fun reorderSection(
            sectionLocalId: Long,
            position: Int,
        ) {
            projects.reorderSection(sectionLocalId, position)
        }

        override suspend fun addTask(
            destination: TaskDestination,
            title: String,
        ) {
            tasks.create(destination, NewTask(title = title))
        }

        override suspend fun moveTask(
            taskLocalId: Long,
            destination: TaskDestination,
        ) {
            tasks.move(taskLocalId, destination)
        }
    }

/** The context screens' writes, made against the shared write path. */
@Singleton
class WriteRepoContextActions
    @Inject
    constructor(
        private val contexts: ContextWriteRepo,
    ) : ContextActions {
        override suspend fun createContext(name: String) {
            contexts.create(name)
        }

        override suspend fun renameContext(
            contextLocalId: Long,
            name: String,
        ) {
            contexts.patch(contextLocalId, name = name)
        }

        override suspend fun deleteContext(contextLocalId: Long) {
            contexts.delete(contextLocalId)
        }
    }
