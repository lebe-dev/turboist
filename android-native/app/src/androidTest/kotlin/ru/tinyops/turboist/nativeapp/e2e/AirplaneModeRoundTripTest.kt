package ru.tinyops.turboist.nativeapp.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.core.sync.write.NewTask
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.core.sync.write.TaskEdit
import java.util.UUID

/**
 * A batch of work done with the radios off arrives on the server exactly once.
 *
 * The batch is chosen to include the case that has nowhere to hide: a task
 * created while offline, and *under it* a second task created while offline,
 * which is then finished — so the child names a parent that has no server id
 * yet, and the completion names a child that has none either. Nothing about that
 * chain can work unless the queue is sent in the order it was written and each
 * answer is fed back onto the rows the next write refers to.
 *
 * The check that matters most is the one for a duplicate. The failure mode of
 * every optimistic write path is the same: the local copy and the server's copy
 * of one task end up as two tasks. So the server is asked how many it has, and
 * the replica is asked whether the id the server assigned landed on the row that
 * was created here rather than on a second one beside it.
 */
@RunWith(AndroidJUnit4::class)
class AirplaneModeRoundTripTest {
    @get:Rule
    val radios = Radios()

    private val settings = HarnessSettings.fromInstrumentation()
    private val control = ServerControl(settings.controlBaseUrl)
    private val device = DeviceUnderTest(settings)
    private val tag = UUID.randomUUID().toString().take(TAG_LENGTH)

    @Before
    fun startFromAFreshDeviceWithASession() =
        runBlocking {
            device.resetToFirstLaunch()
            device.connectAndAuthenticate()
            control.signIn(settings.username, settings.password)
        }

    @Test
    fun workDoneWithNoNetworkReachesTheServerOnceAndInOrder() =
        runBlocking {
            val context = control.createContext("Household $tag")
            val startingProject = control.createProject(context.id, "Renovation $tag")
            val destinationProject = control.createProject(context.id, "Garage $tag")
            device.syncUntilCurrent()
            val startingProjectLocalId =
                requireNotNull(device.database.projects().localIdForServerId(startingProject.id)) {
                    "the project the server holds never reached the replica"
                }
            val destinationProjectLocalId =
                requireNotNull(device.database.projects().localIdForServerId(destinationProject.id)) {
                    "the second project the server holds never reached the replica"
                }

            val parentTitle = "Fit the kitchen $tag"
            val editedTitle = "Fit the kitchen before winter $tag"
            val subtaskTitle = "Measure the worktop $tag"

            radios.cut()

            val parent =
                device.taskWrites.create(
                    TaskDestination.InProject(startingProjectLocalId),
                    NewTask(title = parentTitle),
                )
            val subtask =
                device.taskWrites.create(
                    TaskDestination.SubtaskOf(parent.entityLocalId),
                    NewTask(title = subtaskTitle),
                )
            device.taskWrites.patch(parent.entityLocalId, TaskEdit(title = editedTitle))
            device.taskWrites.complete(subtask.entityLocalId)
            device.taskWrites.move(parent.entityLocalId, TaskDestination.InProject(destinationProjectLocalId))

            assertEquals("the five writes were not all queued", QUEUED_WRITES, device.queueDepth())
            assertTrue(
                "a sync with no network claimed to have reached the server",
                device.sync() is SyncCycleResult.Offline,
            )
            assertEquals(
                "an attempt with no network changed the queue",
                QUEUED_WRITES,
                device.queueDepth(),
            )

            radios.restore()
            device.syncUntilCurrent()

            assertEquals("the queue did not empty", 0, device.queueDepth())
            assertTrue(
                "the server refused a write it should have accepted: ${device.setAside()}",
                device.setAside().isEmpty(),
            )

            val parentRow = requireNotNull(device.database.tasks().byLocalId(parent.entityLocalId))
            val subtaskRow = requireNotNull(device.database.tasks().byLocalId(subtask.entityLocalId))
            val parentServerId = requireNotNull(parentRow.serverId) { "the parent was never given a server id" }
            assertNotNull("the subtask was never given a server id", subtaskRow.serverId)
            assertEquals("the edit made offline did not survive the round trip", editedTitle, parentRow.title)
            assertEquals(
                "the move made offline did not survive the round trip",
                destinationProjectLocalId,
                parentRow.projectLocalId,
            )
            assertEquals(
                "the completion made offline did not survive the round trip",
                TaskStatus.COMPLETED,
                subtaskRow.status,
            )
            assertEquals("the subtask lost its parent", parent.entityLocalId, subtaskRow.parentLocalId)

            // The server's copy resolved onto the row that was created here. Had it
            // arrived as an unknown row instead, the device would now hold two.
            assertEquals(
                "the server's copy of the task became a second row",
                parent.entityLocalId,
                device.database.tasks().localIdForServerId(parentServerId),
            )

            val onTheServer = control.snapshot().tasks
            val parents = onTheServer.filter { it.title == editedTitle }
            assertEquals("the server holds the wrong number of copies of the task", 1, parents.size)
            assertTrue(
                "the server kept a task under the title the edit replaced",
                onTheServer.none { it.title == parentTitle },
            )
            assertEquals(
                "the move did not reach the server",
                destinationProject.id,
                requireNotNull(parents.single().projectId),
            )

            val subtasks = onTheServer.filter { it.title == subtaskTitle }
            assertEquals("the server holds the wrong number of copies of the subtask", 1, subtasks.size)
            assertEquals(
                "the subtask created offline is not under the parent created offline",
                parents.single().id,
                requireNotNull(subtasks.single().parentId),
            )
            assertEquals(
                "the completion did not reach the server",
                TaskStatus.COMPLETED.wire,
                subtasks.single().status,
            )
        }

    private companion object {
        const val TAG_LENGTH: Int = 8
        const val QUEUED_WRITES: Int = 5
    }
}
