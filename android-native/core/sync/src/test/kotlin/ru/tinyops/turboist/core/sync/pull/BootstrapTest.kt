package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.model.RelationType
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Building the replica from nothing, and rebuilding it from a complete copy. */
class BootstrapTest : SyncTest() {
    @Test
    fun `a device that has never synced takes a complete copy and starts from its position`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    epoch = 4,
                    cursor = 812,
                    contexts = listOf(contextJson(7, "Work")),
                    labels = listOf(labelJson(3, "bug")),
                    projects = listOf(projectJson(11, contextId = 7, labels = listOf(labelJson(3, "bug")))),
                    sections = listOf(sectionJson(21, projectId = 11)),
                    tasks =
                        listOf(
                            taskJson(31, "Parent", projectId = 11, sectionId = 21),
                            taskJson(32, "Child", projectId = 11, parentId = 31),
                        ),
                    relations = listOf(relationJson(41, sourceTaskId = 31, targetTaskId = 32)),
                    templates = listOf(templateJson(51, subtasks = listOf(templateSubtaskJson(61, "Step one")))),
                ),
            )

            val result = puller.pull()

            assertTrue(result is PullResult.Applied && result.fromSnapshot, "a first sync copies everything")
            assertEquals(812L, (result as PullResult.Applied).cursor)
            assertEquals(4L, result.epoch)
            assertEquals("/api/v1/sync/snapshot", server.takeRequest().url.encodedPath)

            val position = assertNotNull(db.syncState().get())
            assertEquals(4L, position.epoch)
            assertEquals(812L, position.cursor)

            // Every reference between the rows is the replica's own id, not the
            // server's: the two happen to look alike on a fresh device and must
            // not be confused for each other.
            val context = assertNotNull(db.contexts().byServerId(7))
            val project = assertNotNull(db.projects().byServerId(11))
            assertEquals(context.localId, project.contextLocalId)

            val parent = assertNotNull(db.tasks().byServerId(31))
            val child = assertNotNull(db.tasks().byServerId(32))
            assertEquals(parent.localId, child.parentLocalId)
            assertEquals(project.localId, parent.projectLocalId)
            assertEquals(db.sections().localIdForServerId(21), parent.sectionLocalId)

            val label = assertNotNull(db.labels().byServerId(3))
            assertEquals(listOf(label.localId), db.projects().labelLocalIds(project.localId))

            val edge = db.taskRelations().forTask(parent.localId).single()
            assertEquals(RelationType.BLOCKS, edge.type)
            assertEquals(parent.localId, edge.sourceTaskLocalId)
            assertEquals(child.localId, edge.targetTaskLocalId)

            val templateLocalId = assertNotNull(db.taskTemplates().localIdForServerId(51))
            assertEquals(listOf("Step one"), db.taskTemplates().subtasksOf(templateLocalId).map { it.title })

            val settings = assertNotNull(db.settings().userSettings()).payload
            assertTrue(settings.contains(""""locale":"en""""), "the preferences document is stored as a document")
            assertNotNull(db.settings().appSettings())
            assertNotNull(db.settings().userState())
        }

    @Test
    fun `a later complete copy takes out what it no longer names and keeps the device ids of what it does`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    cursor = 100,
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7)),
                    tasks = listOf(taskJson(31, "Kept", projectId = 11), taskJson(32, "Removed", projectId = 11)),
                ),
            )
            assertTrue(puller.pull().isApplied)
            val keptLocalId = assertNotNull(db.tasks().byServerId(31)).localId

            // The history was replaced, so the catch-up is refused and a second
            // complete copy arrives — this one without task 32.
            enqueueError(409, "sync_epoch_mismatch", """{"epoch":5}""")
            enqueueJson(
                snapshotJson(
                    epoch = 5,
                    cursor = 900,
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7)),
                    tasks = listOf(taskJson(31, "Kept", projectId = 11)),
                ),
            )

            val result = puller.pull()

            assertTrue(result is PullResult.Applied && result.fromSnapshot)
            assertEquals(listOf(31L), serverTaskIds())
            assertEquals(keptLocalId, assertNotNull(db.tasks().byServerId(31)).localId)
            assertEquals(900L, assertNotNull(db.syncState().get()).cursor)
        }

    @Test
    fun `a record whose placement the copy does not carry is dropped rather than failing the copy`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7)),
                    tasks =
                        listOf(
                            taskJson(31, "Placed", projectId = 11),
                            taskJson(32, "Homeless", projectId = 99),
                        ),
                ),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals(listOf(31L), serverTaskIds())
            assertNotNull(db.syncState().get(), "one unplaceable record must not cost the whole copy")
        }

    @Test
    fun `a row created on this device outlives a complete copy that has never heard of it`() =
        runReplicaTest {
            val local = db.contexts().insert(ContextRow(name = "Made offline", createdAt = NOW, updatedAt = NOW))
            queueWrite(ReplicaEntityKind.CONTEXT, local, op = "context.create")

            enqueueJson(snapshotJson(contexts = listOf(contextJson(7))))
            assertTrue(puller.pull().isApplied)

            assertNotNull(db.contexts().byLocalId(local), "a row the server never saw is not a row it deleted")
            assertNull(db.contexts().byServerId(99))
            assertEquals(1, db.outbox().all().size)
        }
}
