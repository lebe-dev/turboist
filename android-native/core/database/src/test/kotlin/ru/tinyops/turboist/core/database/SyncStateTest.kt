package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.SyncStateRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.database.entity.UserStateRow
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/** How far the replica has caught up, and the three documents it keeps whole. */
class SyncStateTest : ReplicaTest() {
    @Test
    fun `a replica that has never synced says so`() =
        runTest {
            assertNull(db.syncState().get(), "there is no position in a history this device has not read")
            assertNull(db.settings().userSettings())
            assertNull(db.settings().appSettings())
            assertNull(db.settings().userState())
        }

    @Test
    fun `catching up moves the one position rather than adding another`() =
        runTest {
            db.syncState().save(SyncStateRow(epoch = 3, cursor = 100, lastSyncAt = NOW))
            db.syncState().save(SyncStateRow(epoch = 3, cursor = 180, lastSyncAt = NOW + 60_000))

            val state = assertNotNull(db.syncState().get())
            assertEquals(3L, state.epoch)
            assertEquals(180L, state.cursor)
            assertEquals(NOW + 60_000, state.lastSyncAt)
        }

    @Test
    fun `starting over discards the position outright`() =
        runTest {
            db.syncState().save(SyncStateRow(epoch = 3, cursor = 100))
            db.syncState().clear()

            assertNull(db.syncState().get(), "a cursor from a history that was replaced means nothing")
        }

    @Test
    fun `a document is stored as it arrived, keys this build knows nothing about included`() =
        runTest {
            val sent = """{"locale":"ru","troikiEnabled":true,"somethingNewer":{"kept":1}}"""
            db.settings().saveUserSettings(UserSettingsRow(payload = sent, updatedAt = NOW))
            db.settings().saveAppSettings(AppSettingsRow(payload = """{"autoLabels":[]}""", updatedAt = NOW))
            db.settings().saveUserState(UserStateRow(payload = """{"activeContextId":4}""", updatedAt = NOW))

            assertEquals(sent, assertNotNull(db.settings().userSettings()).payload)
            assertEquals("""{"autoLabels":[]}""", assertNotNull(db.settings().appSettings()).payload)
            assertEquals("""{"activeContextId":4}""", assertNotNull(db.settings().userState()).payload)

            // Writing again replaces the document; it never accumulates rows.
            db.settings().saveUserSettings(UserSettingsRow(payload = """{"locale":"en"}""", updatedAt = NOW + 1))
            assertEquals("""{"locale":"en"}""", assertNotNull(db.settings().userSettings()).payload)
        }
}
