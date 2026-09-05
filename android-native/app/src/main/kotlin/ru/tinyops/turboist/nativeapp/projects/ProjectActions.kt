package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.NewProject
import ru.tinyops.turboist.core.sync.write.ProjectEdit
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.TaskDestination

/**
 * The writes the project and context screens make.
 *
 * A port onto the shared write path rather than a second copy of it: every one
 * of these applies its change to the replica and queues the request for the
 * server inside one transaction, so a board rearranged in a tunnel is right on
 * screen immediately and right on the server whenever the phone comes back.
 *
 * Task writes a project screen shares with every list — ticking a row off,
 * parking it, planning it for the week — are not restated here. Those screens
 * take the list port for them, so "complete this task" has one meaning in the
 * app rather than two.
 */
interface ProjectActions {
    suspend fun createProject(
        contextLocalId: Long,
        project: NewProject,
    )

    suspend fun editProject(
        projectLocalId: Long,
        edit: ProjectEdit,
    )

    suspend fun setProjectStatus(
        projectLocalId: Long,
        action: ProjectStatusAction,
    )

    suspend fun pinProject(projectLocalId: Long)

    suspend fun unpinProject(projectLocalId: Long)

    /** Puts a project into one of the daily slots, or takes it out of all of them. */
    suspend fun setTroikiCategory(
        projectLocalId: Long,
        category: TroikiCategory?,
    )

    suspend fun deleteProject(projectLocalId: Long)

    suspend fun createSection(
        projectLocalId: Long,
        title: String,
    )

    suspend fun renameSection(
        sectionLocalId: Long,
        title: String,
    )

    suspend fun deleteSection(sectionLocalId: Long)

    /** Moves a board column to [position], counting from the left of the board. */
    suspend fun reorderSection(
        sectionLocalId: Long,
        position: Int,
    )

    /** Writes down a task in one place of the board — a column, or the project itself. */
    suspend fun addTask(
        destination: TaskDestination,
        title: String,
    )

    /** Moves an existing task to another column, or out of every column. */
    suspend fun moveTask(
        taskLocalId: Long,
        destination: TaskDestination,
    )
}

/** The writes the context screens make. */
interface ContextActions {
    suspend fun createContext(name: String)

    suspend fun renameContext(
        contextLocalId: Long,
        name: String,
    )

    suspend fun deleteContext(contextLocalId: Long)
}
