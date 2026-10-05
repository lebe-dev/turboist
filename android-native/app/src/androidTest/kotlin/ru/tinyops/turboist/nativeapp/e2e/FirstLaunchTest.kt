package ru.tinyops.turboist.nativeapp.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import java.util.UUID

/**
 * A device that has never seen this server ends up holding everything on it.
 *
 * This is the scenario every other one depends on, which is why it is checked
 * separately: pointing the app at an address, getting a session out of it, and
 * turning an empty database into a copy of the server are three different things
 * that fail in three different ways, and a later scenario that starts by doing
 * all three would blame the wrong one.
 */
@RunWith(AndroidJUnit4::class)
class FirstLaunchTest {
    @get:Rule
    val radios = Radios()

    private val settings = HarnessSettings.fromInstrumentation()
    private val control = ServerControl(settings.controlBaseUrl)
    private val device = DeviceUnderTest(settings)
    private val tag = UUID.randomUUID().toString().take(TAG_LENGTH)

    @Test
    fun aDeviceWithNothingOnItEndsUpHoldingWhatTheServerHolds() =
        runBlocking {
            device.resetToFirstLaunch()
            assertEquals("a device reset to a first launch still held tasks", 0, device.database.tasks().count())

            device.connectAndAuthenticate()
            control.signIn(settings.username, settings.password)

            val context = control.createContext("Home $tag")
            val project = control.createProject(context.id, "Kitchen $tag")
            val projectTask = control.createProjectTask(project.id, "Order the tiles $tag")
            val inboxTask = control.createInboxTask("Ring the plumber $tag")

            device.syncUntilCurrent()

            assertNotNull(
                "the context the server holds never reached the replica",
                device.database.contexts().byServerId(context.id),
            )
            assertNotNull(
                "the project the server holds never reached the replica",
                device.database.projects().byServerId(project.id),
            )
            assertEquals(
                "Order the tiles $tag",
                device.database.tasks().byServerId(projectTask.id)?.title,
            )
            assertEquals(
                "Ring the plumber $tag",
                device.database.tasks().byServerId(inboxTask.id)?.title,
            )

            // Without a recorded position the next launch would take the whole
            // copy again, which is the difference between a replica and a cache.
            val position = device.database.syncState().get()
            assertNotNull("the replica recorded no position to continue from", position)
            assertTrue("the recorded position was never advanced", requireNotNull(position).cursor > 0)
        }

    private companion object {
        const val TAG_LENGTH: Int = 8
    }
}
