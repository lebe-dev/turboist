package ru.tinyops.turboist.core.sync.drain

import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.sync.write.TaskDestination
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * A move names the whole placement, not just the part the user picked.
 *
 * The server keeps a task's placement as pointers that have to agree — a task in
 * a project is also in that project's context — and it refuses a move that names
 * only the project. The user picked a project, so that is all the queued write
 * records; the rest follows from the replica and is filled in at the moment of
 * sending, exactly like every other id in the request.
 *
 * Getting this wrong is invisible on the device. The move looks right on screen
 * and is refused hours later, when the phone reconnects, as a change the user
 * has long stopped thinking about.
 */
class PlacementTest : DrainTest() {
    @Test
    fun `moving a task into a project names the context that owns it`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject()
            val taskLocalId = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)

            tasks.move(taskLocalId, TaskDestination.InProject(projectLocalId))
            val result = drainer.drain()

            assertTrue(result.isDrained, "the move was not accepted: $result")
            assertEquals(0, result.quarantined)
            val body = movedWith()
            assertTrue(body.contains("\"contextId\":$CONTEXT_SERVER_ID"), "the move named no context: $body")
            assertTrue(body.contains("\"projectId\":$PROJECT_SERVER_ID"), "the move named no project: $body")
        }

    @Test
    fun `moving a task into a section names the project and the context above it`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject()
            val sectionLocalId = givenSyncedSection(projectLocalId)
            val taskLocalId = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)

            tasks.move(taskLocalId, TaskDestination.InSection(sectionLocalId))
            val result = drainer.drain()

            assertTrue(result.isDrained, "the move was not accepted: $result")
            val body = movedWith()
            assertTrue(body.contains("\"contextId\":$CONTEXT_SERVER_ID"), "the move named no context: $body")
            assertTrue(body.contains("\"projectId\":$PROJECT_SERVER_ID"), "the move named no project: $body")
            assertTrue(body.contains("\"sectionId\":$SECTION_SERVER_ID"), "the move named no section: $body")
        }

    @Test
    fun `moving a task under another names where that task sits`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject()
            val parentLocalId = givenTaskInProject(projectLocalId, serverId = PARENT_SERVER_ID)
            val taskLocalId = givenSyncedTask(serverId = 5)
            scripted.givenTask(5)

            tasks.move(taskLocalId, TaskDestination.SubtaskOf(parentLocalId))
            val result = drainer.drain()

            assertTrue(result.isDrained, "the move was not accepted: $result")
            val body = movedWith()
            assertTrue(body.contains("\"contextId\":$CONTEXT_SERVER_ID"), "the move named no context: $body")
            assertTrue(body.contains("\"projectId\":$PROJECT_SERVER_ID"), "the move named no project: $body")
            assertTrue(body.contains("\"parentId\":$PARENT_SERVER_ID"), "the move named no parent: $body")
        }

    /** The body of the one move the server was asked to make. */
    private fun movedWith(): String = scripted.writes().single { it.path.endsWith("/move") }.body

    /** A context and a project inside it, both as an earlier catch-up would have left them. */
    private suspend fun givenSyncedProject(): Long {
        val contextLocalId =
            db.contexts().insert(
                ContextRow(serverId = CONTEXT_SERVER_ID, name = "Work", createdAt = NOW, updatedAt = NOW),
            )
        return db.projects().insert(
            ProjectRow(
                serverId = PROJECT_SERVER_ID,
                contextLocalId = contextLocalId,
                title = "Renovation",
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
    }

    private suspend fun givenSyncedSection(projectLocalId: Long): Long =
        db.sections().insert(
            ProjectSectionRow(
                serverId = SECTION_SERVER_ID,
                projectLocalId = projectLocalId,
                title = "Doing",
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    private suspend fun givenTaskInProject(
        projectLocalId: Long,
        serverId: Long,
    ): Long =
        db.tasks().insert(
            TaskRow(
                serverId = serverId,
                title = "Parent",
                projectLocalId = projectLocalId,
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )

    private companion object {
        const val CONTEXT_SERVER_ID: Long = 11
        const val PROJECT_SERVER_ID: Long = 22
        const val SECTION_SERVER_ID: Long = 33
        const val PARENT_SERVER_ID: Long = 44
    }
}
