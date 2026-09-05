package ru.tinyops.turboist.nativeapp.e2e

import androidx.test.ext.junit.runners.AndroidJUnit4
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.write.TaskEdit
import java.util.UUID

/**
 * A change made offline to a task that is deleted in the meantime is kept where
 * the user can see it, and the rest of the queue still goes out.
 *
 * This is the one conflict a single-user product can actually produce: there is
 * no field-level merge to get wrong, but there is a row that another client can
 * remove while this device is holding an unsent change against it. The rules
 * being checked are that such a change is set aside exactly once — not retried
 * forever, not silently dropped — and that it does not take the writes behind it
 * down with it.
 *
 * The deletion is made while the device really has no network, through the
 * channel the checking client uses. That is what makes it a conflict rather than
 * a sequence: the device cannot possibly have heard about it.
 */
@RunWith(AndroidJUnit4::class)
class DeletedTargetTest {
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
    fun aChangeToATaskTheServerNoLongerHasIsSetAsideAndTheQueueCarriesOn() =
        runBlocking {
            val doomed = control.createInboxTask("Cancelled outing $tag")
            val survivor = control.createInboxTask("Book the tickets $tag")
            device.syncUntilCurrent()

            val doomedLocalId =
                requireNotNull(device.database.tasks().localIdForServerId(doomed.id)) {
                    "the task to be deleted never reached the replica"
                }
            val survivorLocalId =
                requireNotNull(device.database.tasks().localIdForServerId(survivor.id)) {
                    "the second task never reached the replica"
                }

            val survivorEdit = "Book the tickets today $tag"

            radios.cut()
            device.taskWrites.patch(doomedLocalId, TaskEdit(title = "Outing back on $tag"))
            device.taskWrites.patch(survivorLocalId, TaskEdit(title = survivorEdit))
            assertEquals("both edits were not queued", 2, device.queueDepth())

            // Removed underneath a device that cannot hear about it.
            control.deleteTask(doomed.id)

            radios.restore()
            device.syncUntilCurrent()

            assertEquals("the queue did not empty", 0, device.queueDepth())
            val setAside = device.setAside()
            assertEquals("the wrong number of changes were set aside: $setAside", 1, setAside.size)
            assertEquals(
                "the refusal was not filed as the row being gone",
                ApiErrorCodes.TARGET_GONE,
                setAside.single().errorCode,
            )
            assertEquals(
                "the wrong change was set aside",
                doomedLocalId,
                setAside.single().entityLocalId,
            )

            val onTheServer = control.snapshot().tasks
            assertEquals(
                "the write behind the refusal never reached the server",
                1,
                onTheServer.count { it.title == survivorEdit },
            )
            assertTrue(
                "the server still holds the task it was told to delete",
                onTheServer.none { it.id == doomed.id },
            )
            assertNull(
                "the replica kept a task the server no longer has",
                device.database.tasks().byServerId(doomed.id),
            )
        }

    private companion object {
        const val TAG_LENGTH: Int = 8
    }
}
