package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The three preference documents, written against a real database.
 *
 * The claim being checked is a separation, and it is the kind that only fails
 * quietly: the user's own preferences and the installation's rules live in
 * different tables with different owners, and a write that reached the wrong one
 * would look like a working save and be discovered days later as settings that
 * changed by themselves.
 *
 * The second claim is that a document is edited rather than rebuilt. This is a
 * self-hosted product, so the phone and the server are upgraded on different
 * days; a write that rewrote the document from the fields this build knows would
 * destroy every key it does not.
 */
class SettingsWriteTest : WriteTest() {
    @Test
    fun `changing the user's own preferences leaves the installation's rules untouched`() =
        runTest {
            givenAppSettings("""{"autoLabels":[{"mask":"buy","labelIds":[3]}]}""")

            settings.patchUserSettings(PatchUserSettingsRequest(locale = "ru"))

            assertEquals(
                """{"autoLabels":[{"mask":"buy","labelIds":[3]}]}""",
                assertNotNull(db.settings().appSettings()).payload,
                "a preference write reached the installation's rules",
            )
            assertTrue("\"locale\":\"ru\"" in assertNotNull(db.settings().userSettings()).payload)
        }

    @Test
    fun `changing the installation's rules leaves the user's own preferences untouched`() =
        runTest {
            givenUserSettings("""{"locale":"ru","troikiEnabled":true}""")

            settings.putAutoLabels(listOf(AutoLabelRule("buy", listOf(3))))

            assertEquals(
                """{"locale":"ru","troikiEnabled":true}""",
                assertNotNull(db.settings().userSettings()).payload,
                "a rule write reached the user's own preferences",
            )
        }

    @Test
    fun `a preference document written before the interface state existed keeps them apart`() =
        runTest {
            settings.patchUserSettings(PatchUserSettingsRequest(publicView = true))

            assertNull(db.settings().userState(), "a preference write invented an interface state")
            assertNull(db.settings().appSettings(), "a preference write invented an installation rule set")
        }

    @Test
    fun `the two rule lists are stored side by side rather than replacing each other`() =
        runTest {
            settings.putAutoLabels(listOf(AutoLabelRule("buy", listOf(3))))
            settings.putProjectSuggestions(listOf(ProjectSuggestionRule("deploy", listOf(7))))

            val stored = assertNotNull(db.settings().appSettings()).payload
            assertTrue("\"autoLabels\"" in stored, "writing the suggestions dropped the labelling rules")
            assertTrue("\"projectSuggestions\"" in stored)
        }

    @Test
    fun `an installation rule set written by a newer server keeps the keys this build cannot read`() =
        runTest {
            givenAppSettings("""{"autoLabels":[],"somethingNewer":42}""")

            settings.putProjectSuggestions(listOf(ProjectSuggestionRule("deploy", listOf(7))))

            val stored = assertNotNull(db.settings().appSettings()).payload
            assertTrue("somethingNewer" in stored, "an unknown rule was destroyed by an unrelated edit")
        }

    @Test
    fun `each write is queued against the document it belongs to`() =
        runTest {
            settings.patchUserSettings(PatchUserSettingsRequest(locale = "ru"))
            settings.putAutoLabels(listOf(AutoLabelRule("buy", listOf(3))))
            settings.putProjectSuggestions(listOf(ProjectSuggestionRule("deploy", listOf(7))))

            assertEquals(
                listOf(
                    ReplicaEntityKind.USER_SETTINGS,
                    ReplicaEntityKind.APP_SETTINGS,
                    ReplicaEntityKind.APP_SETTINGS,
                ),
                queue().map { it.entity },
            )
            assertEquals(
                listOf(
                    OpNames.SETTINGS_PATCH,
                    OpNames.APP_SETTINGS_AUTO_LABELS,
                    OpNames.APP_SETTINGS_PROJECT_SUGGESTIONS,
                ),
                queue().map { it.op },
            )
        }

    @Test
    fun `a queued preference change carries only the keys that changed`() =
        runTest {
            givenUserSettings("""{"locale":"ru","publicView":true}""")

            settings.patchUserSettings(PatchUserSettingsRequest(locale = "en"))

            val queued = queuedOps().single()
            assertEquals(PatchUserSettingsOp(PatchUserSettingsRequest(locale = "en")), queued)
        }
}
