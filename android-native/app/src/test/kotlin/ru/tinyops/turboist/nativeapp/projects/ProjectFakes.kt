package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectType
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.sync.write.NewProject
import ru.tinyops.turboist.core.sync.write.ProjectEdit
import ru.tinyops.turboist.core.sync.write.ProjectStatusAction
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.WriteRefused
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions

/** A context with nothing set but the two things every context has. */
fun context(
    localId: Long,
    name: String = "context $localId",
): Context = Context(localId = localId, serverId = localId, name = name, createdAt = 0, updatedAt = 0)

/**
 * A project with nothing set but its name and where it lives.
 *
 * Every check below is about one property at a time, so the rest is left at its
 * default: a fixture that spells out a dozen columns hides which one the
 * assertion is actually about.
 */
fun project(
    localId: Long,
    title: String = "project $localId",
    contextLocalId: Long = 1L,
    status: ProjectStatus = ProjectStatus.OPEN,
    type: ProjectType = ProjectType.GENERIC,
    isPinned: Boolean = false,
    pinnedAt: Long? = null,
    troikiCategory: TroikiCategory? = null,
    color: String = "",
): Project =
    Project(
        localId = localId,
        serverId = localId,
        contextLocalId = contextLocalId,
        title = title,
        color = color,
        status = status,
        type = type,
        isPinned = isPinned,
        pinnedAt = pinnedAt,
        troikiCategory = troikiCategory,
        createdAt = 0,
        updatedAt = 0,
    )

/** One column of a board, at the place the caller puts it. */
fun section(
    localId: Long,
    title: String = "section $localId",
    projectLocalId: Long = 1L,
    position: Int = 0,
): ProjectSection =
    ProjectSection(
        localId = localId,
        serverId = localId,
        projectLocalId = projectLocalId,
        title = title,
        position = position,
        createdAt = 0,
        updatedAt = 0,
    )

/**
 * Every write the project screens can make, remembered rather than performed.
 *
 * What each write does to the replica is settled where the write path lives, so
 * a check about a screen only has to say which write it asked for and with what.
 */
class RecordingProjectActions : ProjectActions {
    val calls = mutableListOf<String>()
    var refuseWith: WriteRefused? = null

    private fun record(call: String) {
        calls += call
        refuseWith?.let { throw it }
    }

    override suspend fun createProject(
        contextLocalId: Long,
        project: NewProject,
    ) = record("createProject $contextLocalId ${project.title}")

    override suspend fun editProject(
        projectLocalId: Long,
        edit: ProjectEdit,
    ) = record("editProject $projectLocalId $edit")

    override suspend fun setProjectStatus(
        projectLocalId: Long,
        action: ProjectStatusAction,
    ) = record("setProjectStatus $projectLocalId ${action.segment}")

    override suspend fun pinProject(projectLocalId: Long) = record("pinProject $projectLocalId")

    override suspend fun unpinProject(projectLocalId: Long) = record("unpinProject $projectLocalId")

    override suspend fun setTroikiCategory(
        projectLocalId: Long,
        category: TroikiCategory?,
    ) = record("setTroikiCategory $projectLocalId ${category?.wire}")

    override suspend fun deleteProject(projectLocalId: Long) = record("deleteProject $projectLocalId")

    override suspend fun createSection(
        projectLocalId: Long,
        title: String,
    ) = record("createSection $projectLocalId $title")

    override suspend fun renameSection(
        sectionLocalId: Long,
        title: String,
    ) = record("renameSection $sectionLocalId $title")

    override suspend fun deleteSection(sectionLocalId: Long) = record("deleteSection $sectionLocalId")

    override suspend fun reorderSection(
        sectionLocalId: Long,
        position: Int,
    ) = record("reorderSection $sectionLocalId $position")

    override suspend fun addTask(
        destination: TaskDestination,
        title: String,
    ) = record("addTask $destination $title")

    override suspend fun moveTask(
        taskLocalId: Long,
        destination: TaskDestination,
    ) = record("moveTask $taskLocalId $destination")
}

/** Every write the context screen can make, remembered rather than performed. */
class RecordingContextActions : ContextActions {
    val calls = mutableListOf<String>()

    override suspend fun createContext(name: String) {
        calls += "createContext $name"
    }

    override suspend fun renameContext(
        contextLocalId: Long,
        name: String,
    ) {
        calls += "renameContext $contextLocalId $name"
    }

    override suspend fun deleteContext(contextLocalId: Long) {
        calls += "deleteContext $contextLocalId"
    }
}

/** The four task writes a board shares with every list, remembered rather than performed. */
class RecordingTaskActions : TaskListActions {
    val calls = mutableListOf<String>()
    var refuseWith: WriteRefused? = null

    private fun record(call: String) {
        calls += call
        refuseWith?.let { throw it }
    }

    override suspend fun complete(taskLocalId: Long) = record("complete $taskLocalId")

    override suspend fun uncomplete(taskLocalId: Long) = record("uncomplete $taskLocalId")

    override suspend fun park(taskLocalId: Long) = record("park $taskLocalId")

    override suspend fun planForWeek(taskLocalId: Long) = record("plan $taskLocalId")
}

/** A sync engine that counts requests instead of running one. */
class CountingScheduler : SyncScheduler {
    var requests = 0

    override suspend fun requestSyncNow() {
        requests++
    }
}
