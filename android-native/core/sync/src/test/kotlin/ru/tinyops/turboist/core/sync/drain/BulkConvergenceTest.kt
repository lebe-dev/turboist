package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.core.sync.write.TaskDestination
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * A selection changed with no network, then a server again.
 *
 * The interesting case is a mixed selection: some of it can be finished and some
 * of it cannot, because a task nothing has cleared the way for is refused. The
 * server decides that per item, so a device that decided differently would leave
 * the user looking at a screen the next catch-up quietly corrects.
 *
 * So the claim these cases pin is the strong one: what the device does to a
 * mixed selection offline, and what the server holds once it reconnects, are the
 * same as if the whole selection had been handed to the server in the first
 * place.
 */
class BulkConvergenceTest : DrainTest() {
    @Test
    fun `a mixed selection ticked off offline ends where the server would have put it`() =
        runReplicaTest {
            val blocker = givenSyncedTask(serverId = 10, title = "Sign the contract")
            val blocked = givenSyncedTask(serverId = 11, title = "Start the work")
            val free = givenSyncedTask(serverId = 12, title = "Tidy the desk")
            givenLocalBlocks(blocker = blocker, blocked = blocked)
            scripted.givenTask(10, "Sign the contract")
            scripted.givenTask(11, "Start the work")
            scripted.givenTask(12, "Tidy the desk")
            scripted.givenBlocks(blocker = 10, blocked = 11)

            val written =
                withServerUnreachable {
                    runBlocking {
                        val write = tasks.bulkComplete(listOf(blocked, free))
                        assertEquals(SyncCycleResult.Offline, cycle.runSyncCycle())
                        write
                    }
                }

            assertEquals(listOf(free), written.accepted, "the device leaves out exactly what is in the way")
            assertEquals(mapOf(blocked to listOf(blocker)), written.refused)

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            assertEquals("completed", assertNotNull(scripted.task(12)).status)
            assertEquals("open", assertNotNull(scripted.task(11)).status, "the blocked task was never finished")
            assertEquals("open", assertNotNull(scripted.task(10)).status)
            assertEquals(TaskStatus.COMPLETED, assertNotNull(db.tasks().byLocalId(free)).status)
            assertEquals(TaskStatus.OPEN, assertNotNull(db.tasks().byLocalId(blocked)).status)
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }

    @Test
    fun `the whole selection travels as one request, carrying server ids`() =
        runReplicaTest {
            val selection = (10..14L).map { givenSyncedTask(serverId = it, title = "Task $it") }
            selection.indices.forEach { scripted.givenTask(10L + it, "Task ${10L + it}") }

            withServerUnreachable {
                runBlocking {
                    tasks.bulkComplete(selection)
                    cycle.runSyncCycle()
                }
            }
            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            val sent = scripted.writes()
            assertEquals(1, sent.size, "five tasks ticked off together is one request, not five")
            assertEquals(ScriptedServer.BULK_COMPLETE, sent.single().path)
            assertTrue(
                sent.single().body.contains("\"ids\":[10,11,12,13,14]"),
                "the request must name the server's ids, not the device's: ${sent.single().body}",
            )
            assertContentEquals(
                List(5) { "completed" },
                (10..14L).map { assertNotNull(scripted.task(it)).status },
            )
        }

    @Test
    fun `a group made offline arrives as one parent with the chosen tasks under it`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject()
            val first = givenSyncedTask(serverId = 10, title = "Draft the copy")
            val second = givenSyncedTask(serverId = 11, title = "Pick the photos")
            scripted.givenTask(10, "Draft the copy")
            scripted.givenTask(11, "Pick the photos")

            val write =
                withServerUnreachable {
                    runBlocking {
                        val queued =
                            tasks.group(
                                title = "Launch page",
                                childTaskLocalIds = listOf(first, second),
                                destination = TaskDestination.InProject(projectLocalId),
                            )
                        cycle.runSyncCycle()
                        queued
                    }
                }

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            val sent = scripted.writes()
            assertEquals(1, sent.size, "a group is one request whatever it gathers")
            assertEquals(ScriptedServer.GROUP, sent.single().path)
            val parent = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertNotNull(parent.serverId, "the parent created offline is named by the server's answer")
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }

    /** A project the device already knows the server's name for, with its context above it. */
    private suspend fun givenSyncedProject(): Long {
        val contextLocalId =
            db.contexts().insert(ContextRow(serverId = 1, name = "Work", createdAt = NOW, updatedAt = NOW))
        return db.projects().insert(
            ProjectRow(
                serverId = 2,
                contextLocalId = contextLocalId,
                title = "Website",
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
    }

    /** The device's own record that one task waits on another, as a catch-up would have left it. */
    private suspend fun givenLocalBlocks(
        blocker: Long,
        blocked: Long,
    ) {
        db.taskRelations().insert(
            TaskRelationRow(
                sourceTaskLocalId = blocker,
                targetTaskLocalId = blocked,
                type = RelationType.BLOCKS,
                createdAt = NOW,
            ),
        )
    }
}
