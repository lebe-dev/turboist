package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull

/** A title typed with a closing full stop is stored and sent without it. */
class TrailingDotsTest : WriteTest() {
    @Test
    fun `a new task loses the dots at the end of its title`() =
        runTest {
            val written = tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk..."))

            assertEquals("Buy milk", assertNotNull(db.tasks().byLocalId(written.entityLocalId)).title)
            assertEquals("Buy milk", (queuedOps().last() as CreateTaskOp).body.title)
        }

    @Test
    fun `a new subtask loses the dot at the end of its title`() =
        runTest {
            val project = projects.create(givenContext(), NewProject(title = "Project"))
            val parent = tasks.create(TaskDestination.InProject(project.entityLocalId), NewTask(title = "Parent"))
            val child = tasks.create(TaskDestination.SubtaskOf(parent.entityLocalId), NewTask(title = "Child. "))

            assertEquals("Child", assertNotNull(db.tasks().byLocalId(child.entityLocalId)).title)
        }

    @Test
    fun `dots inside a title and a title made only of dots are kept`() =
        runTest {
            val inner = tasks.create(TaskDestination.Inbox, NewTask(title = "v1.2 release"))
            val dots = tasks.create(TaskDestination.Inbox, NewTask(title = "..."))

            assertEquals("v1.2 release", assertNotNull(db.tasks().byLocalId(inner.entityLocalId)).title)
            assertEquals("...", assertNotNull(db.tasks().byLocalId(dots.entityLocalId)).title)
        }
}
