package ru.tinyops.turboist.core.sync.pull

import androidx.room.Room
import org.junit.Test
import org.robolectric.RuntimeEnvironment
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.sync.SyncMutex
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The property the whole read half stands on: **a device that caught up and a
 * device that started fresh hold the same replica.**
 *
 * If that ever stops being true, one of the two is wrong and no screen can tell
 * which — which is why it is checked against a description of the whole replica
 * rather than against a handful of rows. Deletions are the interesting half: a
 * catch-up learns about them from a tombstone, a fresh copy learns about them by
 * their absence, and those two paths are the ones that drift.
 */
class RoundTripTest : SyncTest() {
    @Test
    fun `catching up and copying afresh end at the same replica`() =
        runReplicaTest {
            enqueueJson(worldBefore())
            assertTrue(puller.pull().isApplied)

            // What happened on the server since: a rename, a new task wearing a
            // new label, and four removals — a task, the edge that pointed at it,
            // a board column and a label nobody uses any more.
            enqueueJson(
                changesJson(
                    listOf(
                        upsert("task", 101, 31, taskJson(31, "Renamed", projectId = 11)),
                        upsert("label", 102, 4, labelJson(4, "urgent")),
                        upsert(
                            "task",
                            103,
                            34,
                            taskJson(34, "Newcomer", projectId = 11, labels = listOf(labelJson(4, "urgent"))),
                        ),
                    ),
                    cursor = 150,
                    hasMore = true,
                ),
            )
            enqueueJson(
                changesJson(
                    listOf(
                        tombstone("task_relation", 151, 41),
                        tombstone("task", 152, 32),
                        tombstone("section", 153, 21),
                        tombstone("label", 154, 3),
                    ),
                    cursor = 200,
                ),
            )

            val result = puller.pull()

            assertEquals(PullResult.Applied(epoch = 1, cursor = 200, changes = 7, fromSnapshot = false), result)
            val caughtUp = describe(db)

            // A second device, seeing the same world for the first time.
            val fresh =
                Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java).build()
            try {
                enqueueJson(worldAfter())
                val freshPuller =
                    SyncPuller(network.sync, fresh, ReplicaApplier(fresh) { NOW }, SyncMutex())
                assertTrue(freshPuller.pull().isApplied)

                assertEquals(describe(fresh), caughtUp)
            } finally {
                fresh.close()
            }

            // And it is the state the story actually describes, not merely two
            // replicas agreeing on the same mistake.
            assertEquals(
                listOf(
                    "context 7 Work",
                    "label 4 urgent",
                    "project 11 Website context=7 labels=[]",
                    "task 31 Renamed project=11 section=null parent=null labels=[]",
                    "task 33 Subtask project=11 section=null parent=31 labels=[]",
                    "task 34 Newcomer project=11 section=null parent=null labels=[4]",
                    "template 51 Weekly review subtasks=[Step one]",
                ),
                caughtUp,
            )
        }

    private fun worldBefore(): String =
        snapshotJson(
            cursor = 100,
            contexts = listOf(contextJson(7, "Work")),
            labels = listOf(labelJson(3, "bug")),
            projects = listOf(projectJson(11, contextId = 7)),
            sections = listOf(sectionJson(21, projectId = 11)),
            tasks =
                listOf(
                    taskJson(31, "Original", projectId = 11),
                    taskJson(32, "Doomed", projectId = 11, sectionId = 21),
                    taskJson(33, "Subtask", projectId = 11, parentId = 31),
                ),
            relations = listOf(relationJson(41, sourceTaskId = 31, targetTaskId = 32)),
            templates = listOf(templateJson(51, subtasks = listOf(templateSubtaskJson(61, "Step one")))),
        )

    private fun worldAfter(): String =
        snapshotJson(
            cursor = 200,
            contexts = listOf(contextJson(7, "Work")),
            labels = listOf(labelJson(4, "urgent")),
            projects = listOf(projectJson(11, contextId = 7)),
            tasks =
                listOf(
                    taskJson(31, "Renamed", projectId = 11),
                    taskJson(33, "Subtask", projectId = 11, parentId = 31),
                    taskJson(34, "Newcomer", projectId = 11, labels = listOf(labelJson(4, "urgent"))),
                ),
            templates = listOf(templateJson(51, subtasks = listOf(templateSubtaskJson(61, "Step one")))),
        )

    /**
     * The whole replica as text, addressed by server ids.
     *
     * Device ids are deliberately left out: two devices that hold the same
     * records will have numbered them differently, and the claim being checked is
     * about the records.
     */
    private suspend fun describe(target: TurboistDatabase): List<String> {
        val lines = mutableListOf<String>()
        target.contexts().knownServerIds().sorted().forEach { id ->
            lines += "context $id ${assertNotNull(target.contexts().byServerId(id)).name}"
        }
        target.labels().knownServerIds().sorted().forEach { id ->
            lines += "label $id ${assertNotNull(target.labels().byServerId(id)).name}"
        }
        target.projects().knownServerIds().sorted().forEach { id ->
            val row = assertNotNull(target.projects().byServerId(id))
            val context = target.contexts().byLocalId(row.contextLocalId)?.serverId
            val labels =
                target.projects().labelLocalIds(
                    row.localId,
                ).mapNotNull { target.labels().byLocalId(it)?.serverId }
            lines += "project $id ${row.title} context=$context labels=${labels.sorted()}"
        }
        target.sections().knownServerIds().sorted().forEach { id ->
            val row =
                assertNotNull(target.sections().byLocalId(assertNotNull(target.sections().localIdForServerId(id))))
            val project = target.projects().byLocalId(row.projectLocalId)?.serverId
            lines += "section $id ${row.title} project=$project"
        }
        target.tasks().knownServerIds().sorted().forEach { id ->
            val row = assertNotNull(target.tasks().byServerId(id))
            val project = row.projectLocalId?.let { target.projects().byLocalId(it)?.serverId }
            val section = row.sectionLocalId?.let { target.sections().byLocalId(it)?.serverId }
            val parent = row.parentLocalId?.let { target.tasks().byLocalId(it)?.serverId }
            val labels =
                target.tasks().labelsOf(row.localId).mapNotNull { target.labels().byLocalId(it.labelLocalId)?.serverId }
            lines += "task $id ${row.title} project=$project section=$section parent=$parent labels=${labels.sorted()}"
        }
        target.taskRelations().knownServerIds().sorted().forEach { id ->
            val row =
                assertNotNull(
                    target.taskRelations().byLocalId(assertNotNull(target.taskRelations().localIdForServerId(id))),
                )
            val source = target.tasks().byLocalId(row.sourceTaskLocalId)?.serverId
            val target2 = target.tasks().byLocalId(row.targetTaskLocalId)?.serverId
            lines += "relation $id ${row.type.wire} $source->$target2"
        }
        target.taskTemplates().knownServerIds().sorted().forEach { id ->
            val localId = assertNotNull(target.taskTemplates().localIdForServerId(id))
            val row = assertNotNull(target.taskTemplates().byLocalId(localId))
            val subtasks = target.taskTemplates().subtasksOf(localId).map { it.title }
            lines += "template $id ${row.name} subtasks=$subtasks"
        }
        return lines
    }
}
