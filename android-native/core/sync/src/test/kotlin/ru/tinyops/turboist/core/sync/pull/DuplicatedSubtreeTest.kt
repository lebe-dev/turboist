package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * A copy made on the server arrives as ordinary changes: the new root and each
 * copied subtask, every one carrying the labels the server gave it.
 *
 * The device never builds the copy itself, so what it shows is exactly what these
 * rows say. A subtask that came in without its labels would read as a copy that
 * lost its tags, which is the shape the web client once showed.
 */
class DuplicatedSubtreeTest : SyncTest() {
    @Test
    fun `a copied subtree keeps the labels of every row it arrives with`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    cursor = 100,
                    contexts = listOf(contextJson(7, "Work")),
                    labels = listOf(labelJson(3, "engineer"), labelJson(4, "quality")),
                    tasks =
                        listOf(
                            taskJson(31, "Goal", contextId = 7, labels = listOf(labelJson(3, "engineer"))),
                            taskJson(
                                32,
                                "Review",
                                contextId = 7,
                                parentId = 31,
                                labels = listOf(labelJson(3, "engineer"), labelJson(4, "quality")),
                            ),
                        ),
                ),
            )
            assertTrue(puller.pull().isApplied)

            enqueueJson(
                changesJson(
                    listOf(
                        upsert(
                            "task",
                            101,
                            41,
                            taskJson(41, "Goal (2)", contextId = 7, labels = listOf(labelJson(3, "engineer"))),
                        ),
                        upsert(
                            "task",
                            102,
                            42,
                            taskJson(
                                42,
                                "Review",
                                contextId = 7,
                                parentId = 41,
                                labels = listOf(labelJson(3, "engineer"), labelJson(4, "quality")),
                            ),
                        ),
                        upsert("task", 103, 43, taskJson(43, "Untagged", contextId = 7, parentId = 41)),
                    ),
                    cursor = 103,
                ),
            )
            assertTrue(puller.pull().isApplied)

            assertEquals(
                mapOf(
                    41L to Triple("Goal (2)", null, listOf(3L)),
                    42L to Triple("Review", 41L, listOf(3L, 4L)),
                    43L to Triple("Untagged", 41L, emptyList()),
                ),
                listOf(41L, 42L, 43L).associateWith { describeTask(it) },
            )
        }

    @Test
    fun `a copied subtask delivered ahead of its root still lands with its labels`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    cursor = 100,
                    contexts = listOf(contextJson(7, "Work")),
                    labels = listOf(labelJson(3, "engineer")),
                ),
            )
            assertTrue(puller.pull().isApplied)

            enqueueJson(
                changesJson(
                    listOf(
                        upsert(
                            "task",
                            101,
                            42,
                            taskJson(
                                42,
                                "Review",
                                contextId = 7,
                                parentId = 41,
                                labels = listOf(labelJson(3, "engineer")),
                            ),
                        ),
                        upsert("task", 102, 41, taskJson(41, "Goal (2)", contextId = 7)),
                    ),
                    cursor = 102,
                ),
            )
            assertTrue(puller.pull().isApplied)

            assertEquals(Triple("Review", 41L, listOf(3L)), describeTask(42))
        }

    private suspend fun describeTask(serverId: Long): Triple<String, Long?, List<Long>> {
        val row = assertNotNull(db.tasks().byServerId(serverId))
        val parent = row.parentLocalId?.let { db.tasks().byLocalId(it)?.serverId }
        val labels =
            db.tasks().labelsOf(row.localId)
                .mapNotNull { db.labels().byLocalId(it.labelLocalId)?.serverId }
                .sorted()
        return Triple(row.title, parent, labels)
    }
}
