package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.model.INBOX_ID
import ru.tinyops.turboist.core.model.Priority
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * Gathering loose tasks under a new parent.
 *
 * Grouping is the one action that both creates something and rewrites what was
 * already there, so it has more to get wrong than any other bulk action: where
 * the new parent lands, what the children look like afterwards, and how many
 * requests the whole thing turns into. All three are checked against a real
 * replica, because all three are claims about rows.
 */
class GroupTest : WriteTest() {
    @Test
    fun `the new parent lands where it was pointed and the children follow it there`() =
        runTest {
            val contextLocalId = givenContext()
            val projectLocalId = givenProject(contextLocalId)
            val sectionLocalId = givenSection(projectLocalId)
            val first = givenTask(title = "Draft the copy", serverId = 11, inboxId = INBOX_ID)
            val second = givenTask(title = "Pick the photos", serverId = 12, inboxId = INBOX_ID)

            val write =
                tasks.group(
                    title = "Launch page",
                    childTaskLocalIds = listOf(first, second),
                    destination = TaskDestination.InSection(sectionLocalId),
                )

            val parent = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertEquals(projectLocalId, parent.projectLocalId)
            assertEquals(sectionLocalId, parent.sectionLocalId)
            assertNull(parent.inboxId, "a group is structure, and the inbox holds none")
            assertTrue(parent.isComplex, "a task with work hanging under it is a piece of work in parts")
            for (childLocalId in listOf(first, second)) {
                val child = assertNotNull(db.tasks().byLocalId(childLocalId))
                assertEquals(write.entityLocalId, child.parentLocalId)
                assertEquals(projectLocalId, child.projectLocalId)
                assertEquals(sectionLocalId, child.sectionLocalId)
                assertNull(child.inboxId, "a child of a group is no longer loose capture")
            }
        }

    @Test
    fun `the children are rewritten to carry the parent's labels and priority`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val urgent = givenLabel(name = "urgent", serverId = 21)
            val chore = givenLabel(name = "chore", serverId = 22)
            val child = givenTask(title = "Book the room", serverId = 11, projectLocalId = projectLocalId)
            db.tasks().setLabels(child, listOf(chore), NOW)
            db.tasks().update(assertNotNull(db.tasks().byLocalId(child)).copy(priority = Priority.LOW))

            val write =
                tasks.group(
                    title = "Offsite",
                    childTaskLocalIds = listOf(child),
                    destination = TaskDestination.InProject(projectLocalId),
                    priority = Priority.HIGH,
                    labels = listOf("urgent"),
                )

            val parent = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertEquals(Priority.HIGH, parent.priority)
            val adopted = assertNotNull(db.tasks().byLocalId(child))
            assertEquals(
                Priority.HIGH,
                adopted.priority,
                "a child of a group takes the group's priority, not the one it had while it was loose",
            )
            assertContentEquals(
                listOf(urgent),
                db.tasks().labelsOf(child).map { it.labelLocalId },
                "the group's labels replace the child's rather than being added to them",
            )
        }

    @Test
    fun `however many tasks are gathered, the server is asked once`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val children = (11..18L).map { givenTask(title = "Step $it", serverId = it) }

            tasks.group(
                title = "The whole job",
                childTaskLocalIds = children,
                destination = TaskDestination.InProject(projectLocalId),
            )

            val op = assertNotNull(queuedOps().singleOrNull() as? GroupTasksOp)
            assertContentEquals(children, op.childTaskLocalIds)
            assertEquals(TaskDestination.InProject(projectLocalId), op.destination)
        }

    @Test
    fun `a task named twice is gathered once`() =
        runTest {
            val projectLocalId = givenProject(givenContext())
            val child = givenTask(title = "Only once", serverId = 11)

            val write =
                tasks.group(
                    title = "Group",
                    childTaskLocalIds = listOf(child, child, child),
                    destination = TaskDestination.InProject(projectLocalId),
                )

            val op = assertNotNull(queuedOps().single() as? GroupTasksOp)
            assertContentEquals(listOf(child), op.childTaskLocalIds)
            assertEquals(
                write.entityLocalId,
                assertNotNull(db.tasks().byLocalId(child)).parentLocalId,
            )
        }

    @Test
    fun `the inbox and another task are both refused as the home of a group`() =
        runTest {
            val child = givenTask(title = "Loose", serverId = 11)
            val other = givenTask(title = "Something else", serverId = 12)

            assertFailsWith<WriteRefused.Placement> {
                tasks.group("Group", listOf(child), TaskDestination.Inbox)
            }
            assertFailsWith<WriteRefused.Placement> {
                tasks.group("Group", listOf(child), TaskDestination.SubtaskOf(other))
            }
            assertTrue(queue().isEmpty(), "a refused group must leave nothing queued")
        }

    private suspend fun givenSection(
        projectLocalId: Long,
        title: String = "Doing",
        serverId: Long? = 31,
    ): Long =
        db.sections().insert(
            ProjectSectionRow(
                serverId = serverId,
                projectLocalId = projectLocalId,
                title = title,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
}
