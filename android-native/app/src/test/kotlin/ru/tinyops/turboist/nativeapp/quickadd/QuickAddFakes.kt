package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination

/** One task the sheet asked for, recorded so a test can say what was written down. */
data class CreateCall(
    val destination: TaskDestination,
    val task: NewTask,
)

/**
 * The write path as far as the capture sheet can tell.
 *
 * It records what it was asked for and can be told to refuse from a given call
 * onwards, which is all a test of the sheet needs: what the write path then does
 * to the replica is settled where the write path lives.
 */
class RecordingQuickAddActions(
    /** The call, counting from one, at which every further create fails. */
    var failFrom: Int = Int.MAX_VALUE,
) : QuickAddActions {
    val calls = mutableListOf<CreateCall>()

    override suspend fun create(
        destination: TaskDestination,
        task: NewTask,
    ) {
        if (calls.size + 1 >= failFrom) throw IllegalStateException("the write path is unavailable")
        calls += CreateCall(destination, task)
    }
}

/** The remembered project order, in memory. */
class FakeRecentProjects(
    initial: List<Long> = emptyList(),
) : RecentProjects {
    private val order = MutableStateFlow(initial)

    /** The order as it stands, for a case that asserts about it. */
    val remembered: List<Long> get() = order.value

    override fun observe(): Flow<List<Long>> = order

    override suspend fun remember(projectLocalId: Long) {
        order.value = withRecentProject(order.value, projectLocalId)
    }

    override suspend fun clear() {
        order.value = emptyList()
    }
}

/**
 * A project, with only the fields a picker or a suggestion rule reads spelled
 * out. Reused by every case here rather than rebuilt per test.
 */
fun project(
    localId: Long,
    title: String,
    serverId: Long? = localId,
    status: ProjectStatus = ProjectStatus.OPEN,
    isPinned: Boolean = false,
): Project =
    Project(
        localId = localId,
        serverId = serverId,
        contextLocalId = 1,
        title = title,
        status = status,
        isPinned = isPinned,
        createdAt = 0,
        updatedAt = 0,
    )

/** A label, named and addressable, which is all the title rules read. */
fun label(
    localId: Long,
    name: String,
    serverId: Long? = localId,
): Label =
    Label(
        localId = localId,
        serverId = serverId,
        name = name,
        createdAt = 0,
        updatedAt = 0,
    )
