package ru.tinyops.turboist.core.sync.drain

import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.sync.SyncCycleResult
import ru.tinyops.turboist.core.sync.write.TemplateDraft
import ru.tinyops.turboist.core.sync.write.TemplateSubtaskDraft
import ru.tinyops.turboist.core.sync.write.TemplateWriteRepo
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * A template used with no server, and the tree it made once there was one.
 *
 * The claim under test is the one the feature is worth anything for: what the
 * user saw on the plane is what the server ends up holding — the same tasks,
 * hanging off each other the same way, in one copy. Both ends are real, so the
 * two things that would break it are visible here and nowhere else: a tree drawn
 * on the device that the server's answer cannot be matched to, and a second copy
 * arriving with the next catch-up.
 */
class TemplateRoundTripTest : DrainTest() {
    @Test
    fun `a template used with no network drains into the same tree on the server`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject(serverId = 7)
            val templateLocalId = givenSyncedTemplate(serverId = 5, name = "Release", lines = listOf("Tag", "Notes"))
            scripted.givenProject(id = 7)
            scripted.givenTemplate(id = 5, name = "Release", subtaskTitles = listOf("Tag", "Notes"))

            val write =
                withServerUnreachable {
                    runBlocking {
                        val made = templates().instantiate(templateLocalId, projectLocalId)
                        // The tree is there before anything has been sent: that is
                        // what makes it usable on a plane.
                        assertEquals("Release", assertNotNull(db.tasks().byLocalId(made.entityLocalId)).title)
                        assertContentEquals(
                            listOf("Tag", "Notes"),
                            db.tasks().subtasksOf(made.entityLocalId).map { it.title },
                        )
                        assertEquals(SyncCycleResult.Offline, cycle.runSyncCycle())
                        made
                    }
                }

            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            val root = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            val rootOnServer = assertNotNull(scripted.tasks().firstOrNull { it.title == "Release" })
            assertEquals(rootOnServer.id, root.serverId, "the answer named the row that was already on screen")
            assertContentEquals(
                listOf("Tag", "Notes"),
                scripted.tasks().filter { it.parentId == rootOnServer.id }.map { it.title },
                "the server holds the same checklist under the same task",
            )
            assertEquals(
                db.tasks().subtasksOf(root.localId).mapNotNull { it.serverId },
                scripted.tasks().filter { it.parentId == rootOnServer.id }.map { it.id },
                "each line of the checklist is the row the device drew for it",
            )
            assertEquals(3, db.tasks().count(), "one copy of each, not a second set from the catch-up")
            assertEquals(3, scripted.tasks().size)
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }

    @Test
    fun `the tree survives the process that drew it, and drains from the next one`() =
        runReplicaTest {
            val projectLocalId = givenSyncedProject(serverId = 7)
            val templateLocalId = givenSyncedTemplate(serverId = 5, name = "Release", lines = listOf("Tag"))
            scripted.givenProject(id = 7)
            scripted.givenTemplate(id = 5, name = "Release", subtaskTitles = listOf("Tag"))
            val write =
                withServerUnreachable {
                    runBlocking { templates().instantiate(templateLocalId, projectLocalId) }
                }

            // Nothing of what was queued lives in memory: a drainer built fresh
            // over the same replica is what a relaunched app has.
            newDrainer().drain()

            val root = assertNotNull(db.tasks().byLocalId(write.entityLocalId))
            assertNotNull(root.serverId, "a relaunch must be able to send what the last run wrote down")
            assertEquals(2, scripted.tasks().size)
            assertTrue(queue().isEmpty())
        }

    @Test
    fun `a task split with no network arrives as the pieces and not the original`() =
        runReplicaTest {
            val sourceLocalId = givenSyncedTask(serverId = 31, title = "Draft outline")
            scripted.givenTask(id = 31, title = "Draft outline")

            withServerUnreachable {
                runBlocking { tasks.decompose(sourceLocalId, listOf("Review with team", "Publish")) }
            }
            assertEquals(SyncCycleResult.Synced, cycle.runSyncCycle())

            assertContentEquals(listOf("Review with team", "Publish"), scripted.tasks().map { it.title })
            assertNull(scripted.task(31), "the task the pieces replaced is gone on the server too")
            assertEquals(2, db.tasks().count(), "one copy of each piece")
            val first = assertNotNull(db.tasks().byLocalId(sourceLocalId))
            assertEquals("Review with team", first.title)
            assertEquals(
                scripted.tasks().first().id,
                first.serverId,
                "the row the work was in is now the first piece, under the name the server gave it",
            )
            assertTrue(queue().isEmpty())
            assertTrue(setAside().isEmpty())
        }

    // --- fixtures -------------------------------------------------------------

    /** The write path for templates, over the same replica and queue as the task one. */
    private fun templates(): TemplateWriteRepo = TemplateWriteRepo(db, writer)

    private suspend fun givenSyncedProject(serverId: Long): Long {
        val contextLocalId =
            db.contexts().insert(ContextRow(serverId = serverId, name = "Work", createdAt = NOW, updatedAt = NOW))
        return db.projects().insert(
            ProjectRow(
                serverId = serverId,
                contextLocalId = contextLocalId,
                title = "Website",
                createdAt = NOW,
                updatedAt = NOW,
            ),
        )
    }

    /** A template the device already knows the server's name for, checklist and all. */
    private suspend fun givenSyncedTemplate(
        serverId: Long,
        name: String,
        lines: List<String>,
    ): Long {
        val localId =
            TemplateWriteRepo(db, writer)
                .create(TemplateDraft(name = name, subtasks = lines.map { TemplateSubtaskDraft(title = it) }))
                .entityLocalId
        db.taskTemplates().assignServerId(localId, serverId, NOW)
        // The write that created it has served its purpose as a fixture; the
        // cases here are about what happens after a template exists.
        db.outbox().clear()
        return localId
    }
}
