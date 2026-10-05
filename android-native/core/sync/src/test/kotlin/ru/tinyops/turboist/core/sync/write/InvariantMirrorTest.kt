package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The rules the device repeats for itself: what a new task gets labelled with,
 * how much fits on the pinned shelf, how many projects fit in a slot of the
 * daily plan, and the priority a slot fixes for the work inside it.
 *
 * All three come from documents the server sent and this device stored. None of
 * them is a second implementation of the domain — each exists because getting it
 * wrong on screen and correcting it hours later is worse than answering now.
 */
class InvariantMirrorTest : WriteTest() {
    @Test
    fun `a new task picks up the labels the installation's rules attach`() =
        runTest {
            val labelLocalId = givenLabel(name = "bug", serverId = 7)
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[7],"ignoreCase":true}]}""")

            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "Fix the login form"))

            assertEquals(
                listOf(labelLocalId),
                db.tasks().labelsOf(written.entityLocalId).map { it.labelLocalId },
            )
        }

    @Test
    fun `a rule the user overrode does not put its label back`() =
        runTest {
            givenLabel(name = "bug", serverId = 7)
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[7]}]}""")

            val written =
                tasks.create(
                    TaskDestination.Inbox,
                    NewTask(title = "fix the login form", removedAutoLabels = listOf("bug")),
                )

            assertTrue(db.tasks().labelsOf(written.entityLocalId).isEmpty())
        }

    @Test
    fun `a rule naming a label this device has never seen attaches nothing`() =
        runTest {
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[404]}]}""")

            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "fix it"))

            // The server applies its own rules authoritatively a moment later.
            // Inventing a label here would create a second one with the same name.
            assertTrue(db.tasks().labelsOf(written.entityLocalId).isEmpty())
        }

    @Test
    fun `a case-sensitive rule does not match a different casing`() =
        runTest {
            givenLabel(name = "bug", serverId = 7)
            givenAppSettings("""{"autoLabels":[{"mask":"Fix","labelIds":[7],"ignoreCase":false}]}""")

            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "fix the login form"))

            assertTrue(db.tasks().labelsOf(written.entityLocalId).isEmpty())
        }

    @Test
    fun `taking a rule's label off an existing task keeps it off`() =
        runTest {
            givenLabel(name = "bug", serverId = 7)
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[7],"ignoreCase":true}]}""")
            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "Fix the login form"))

            tasks.patch(written.entityLocalId, TaskEdit(labels = emptyList()))

            assertTrue(
                db.tasks().labelsOf(written.entityLocalId).isEmpty(),
                "an unticked automatic label must not be re-attached on the spot",
            )
            val sent = queuedOps().filterIsInstance<PatchTaskOp>().single()
            assertEquals(
                listOf("bug"),
                sent.patch.removedAutoLabels,
                "the server re-runs the same rules, so the refusal has to travel with the write",
            )
        }

    @Test
    fun `a label a rule only now matches is not treated as one the user refused`() =
        runTest {
            val labelLocalId = givenLabel(name = "bug", serverId = 7)
            givenAppSettings("""{"autoLabels":[{"mask":"fix","labelIds":[7],"ignoreCase":true}]}""")
            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "Look at the login form"))
            assertTrue(db.tasks().labelsOf(written.entityLocalId).isEmpty())

            tasks.patch(written.entityLocalId, TaskEdit(title = "Fix the login form", labels = emptyList()))

            assertEquals(
                listOf(labelLocalId),
                db.tasks().labelsOf(written.entityLocalId).map { it.labelLocalId },
                "a rule that starts matching attaches a label the user has never seen",
            )
            assertEquals(null, queuedOps().filterIsInstance<PatchTaskOp>().single().patch.removedAutoLabels)
        }

    @Test
    fun `the pinned shelf holds as many as the user said`() =
        runTest {
            givenUserSettings("""{"maxPinnedTasks":2}""")
            val first = givenTask(title = "One", serverId = 1)
            val second = givenTask(title = "Two", serverId = 2)
            val third = givenTask(title = "Three", serverId = 3)

            tasks.pin(first)
            tasks.pin(second)
            val refused = assertFailsWith<WriteRefused.PinLimitReached> { tasks.pin(third) }

            assertEquals(2, refused.limit)
            assertEquals(false, assertNotNull(db.tasks().byLocalId(third)).isPinned)
            assertEquals(2, queue().size, "the refused pin must not be queued")
        }

    @Test
    fun `pinning something already pinned is not a new place on the shelf`() =
        runTest {
            givenUserSettings("""{"maxPinnedTasks":1}""")
            val taskLocalId = givenTask(title = "One", serverId = 1)

            tasks.pin(taskLocalId)
            tasks.pin(taskLocalId)

            assertEquals(true, assertNotNull(db.tasks().byLocalId(taskLocalId)).isPinned)
        }

    @Test
    fun `a full slot of the daily plan takes no more projects`() =
        runTest {
            val contextLocalId = givenContext()
            val occupants =
                (1..ReplicaRules.TROIKI_SLOT_BASE_CAPACITY).map { index ->
                    givenProject(contextLocalId, title = "Project $index", serverId = index.toLong())
                }
            for (localId in occupants) projects.setTroikiCategory(localId, TroikiCategory.IMPORTANT)
            val extra = givenProject(contextLocalId, title = "One too many", serverId = 99)

            val refused =
                assertFailsWith<WriteRefused.TroikiSlotFull> {
                    projects.setTroikiCategory(extra, TroikiCategory.IMPORTANT)
                }

            assertEquals(TroikiCategory.IMPORTANT, refused.category)
            assertEquals(null, assertNotNull(db.projects().byLocalId(extra)).troikiCategory)
        }

    @Test
    fun `a finished project cannot take a place in the daily plan`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            projects.setStatus(projectLocalId, ProjectStatusAction.COMPLETE)

            assertFailsWith<WriteRefused.Placement> {
                projects.setTroikiCategory(projectLocalId, TroikiCategory.MEDIUM)
            }
            assertEquals(
                ProjectStatus.COMPLETED,
                assertNotNull(db.projects().byLocalId(projectLocalId)).status,
            )
        }

    @Test
    fun `resetting the daily plan empties every slot`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            projects.setTroikiCategory(projectLocalId, TroikiCategory.REST)

            troiki.reset()

            assertEquals(null, assertNotNull(db.projects().byLocalId(projectLocalId)).troikiCategory)
            assertEquals(OpNames.TROIKI_RESET, queue().last().op)
        }

    @Test
    fun `taking a place in the daily plan re-pins the open work in the project`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val open = givenTask(title = "Open", serverId = 1, projectLocalId = projectLocalId)
            val subtask =
                givenTask(title = "Under it", serverId = 2, projectLocalId = projectLocalId, parentLocalId = open)
            val done =
                givenTask(
                    title = "Already done",
                    serverId = 3,
                    projectLocalId = projectLocalId,
                    priority = Priority.NONE,
                    status = TaskStatus.COMPLETED,
                )

            projects.setTroikiCategory(projectLocalId, TroikiCategory.IMPORTANT)

            assertEquals(Priority.HIGH, assertNotNull(db.tasks().byLocalId(open)).priority)
            assertEquals(Priority.HIGH, assertNotNull(db.tasks().byLocalId(subtask)).priority)
            // Finished work is the record of what was done and is left as it was.
            assertEquals(Priority.NONE, assertNotNull(db.tasks().byLocalId(done)).priority)
        }

    @Test
    fun `each slot of the daily plan fixes its own priority`() =
        runTest {
            val contextLocalId = givenContext()
            val expected =
                mapOf(
                    TroikiCategory.IMPORTANT to Priority.HIGH,
                    TroikiCategory.MEDIUM to Priority.MEDIUM,
                    TroikiCategory.REST to Priority.LOW,
                )
            for ((index, entry) in expected.entries.withIndex()) {
                val serverId = index.toLong() + 1
                val projectLocalId =
                    givenProject(contextLocalId, title = "Project $serverId", serverId = serverId)
                val taskLocalId =
                    givenTask(title = "Task $serverId", serverId = serverId, projectLocalId = projectLocalId)

                projects.setTroikiCategory(projectLocalId, entry.key)

                assertEquals(entry.value, assertNotNull(db.tasks().byLocalId(taskLocalId)).priority)
            }
        }

    @Test
    fun `a task written into a project in the daily plan takes that priority`() =
        runTest {
            val projectLocalId =
                givenProject(givenContext(), troikiCategory = TroikiCategory.MEDIUM)

            val written =
                tasks.create(
                    TaskDestination.InProject(projectLocalId),
                    NewTask(title = "Draft the note", priority = Priority.NONE),
                )

            assertEquals(Priority.MEDIUM, assertNotNull(db.tasks().byLocalId(written.entityLocalId)).priority)
        }

    @Test
    fun `a task moved into a project in the daily plan takes that priority`() =
        runTest {
            val projectLocalId =
                givenProject(givenContext(), troikiCategory = TroikiCategory.REST)
            val taskLocalId = givenTask(title = "Move me", serverId = 5, priority = Priority.HIGH)

            tasks.move(taskLocalId, TaskDestination.InProject(projectLocalId))

            assertEquals(Priority.LOW, assertNotNull(db.tasks().byLocalId(taskLocalId)).priority)
        }

    @Test
    fun `preferences are edited without losing the keys this build does not know`() =
        runTest {
            givenUserSettings("""{"locale":"ru","somethingNewer":42}""")

            settings.patchUserSettings(
                ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest(locale = "en"),
            )

            val stored = assertNotNull(db.settings().userSettings()).payload
            assertTrue("somethingNewer" in stored, "an unknown preference was destroyed by an unrelated edit")
            assertTrue("\"locale\":\"en\"" in stored)
        }
}
