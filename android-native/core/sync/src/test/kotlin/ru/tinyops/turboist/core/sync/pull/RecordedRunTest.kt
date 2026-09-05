package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import ru.tinyops.turboist.core.model.WireTime
import ru.tinyops.turboist.core.sync.write.OutboxWriter
import ru.tinyops.turboist.core.sync.write.RecurrenceAdvancer
import ru.tinyops.turboist.core.sync.write.ReplicaRules
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * A repeating task ticked off with no connection, and what the catch-up makes
 * of it afterwards.
 *
 * Ticking one off leaves the task open on its next date and writes down the run
 * that was finished, so the history has something to show immediately. The
 * server writes down the same run when the completion reaches it. Both records
 * describe one event, and the user must end up with one entry in their history
 * rather than two — which is the whole of what these cases are about.
 */
class RecordedRunTest : SyncTest() {
    /** A replica holding one repeating task, at position 100. */
    private suspend fun seededWithRepeatingTask(): Long {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7)),
                projects = listOf(projectJson(11, contextId = 7)),
                tasks = listOf(taskJson(31, "Water the plants", projectId = 11)),
            ),
        )
        assertTrue(puller.pull().isApplied)
        server.takeRequest()
        val task = assertNotNull(db.tasks().byServerId(31))
        db.tasks().update(task.copy(recurrenceRule = "FREQ=DAILY", dueAt = DUE))
        return task.localId
    }

    /**
     * Ticks the task off the way a screen does, then empties the queue.
     *
     * Emptying it stands in for the send that always comes first: the engine
     * drains before it reads, so by the time a page arrives the write is no
     * longer pending. Leaving it there would have the applier protect the row
     * from an incoming change, which is a different case entirely.
     */
    private suspend fun tickOffOfflineAndSend(taskLocalId: Long) {
        val writes =
            TaskWriteRepo(
                db = db,
                writer = OutboxWriter(db, { NOW }, { "op-1" }),
                rules = ReplicaRules(db),
                recurrence = RecurrenceAdvancer(ZONE),
                zone = ZONE,
            )
        writes.complete(taskLocalId)
        db.outbox().all().forEach { db.outbox().deleteById(it.id) }
    }

    @Test
    fun `the server's record of a run replaces the device's instead of doubling it`() =
        runReplicaTest {
            val taskLocalId = seededWithRepeatingTask()
            tickOffOfflineAndSend(taskLocalId)
            val recorded = assertNotNull(db.tasks().unnamedRecurrenceCompletion(taskLocalId, NOW))

            enqueueJson(
                changesJson(
                    listOf(
                        upsert("task", 101, 31, taskJson(31, "Water the plants", projectId = 11)),
                        upsert(
                            "task",
                            102,
                            32,
                            taskJson(
                                32,
                                "Water the plants",
                                projectId = 11,
                                sourceTaskId = 31,
                                status = "completed",
                                completedAt = WIRE_NOW,
                            ),
                        ),
                    ),
                    cursor = 200,
                ),
            )
            assertTrue(puller.pull().isApplied)

            assertEquals(
                recorded.localId,
                assertNotNull(db.tasks().byServerId(32)).localId,
                "the run the device wrote down is the run the server wrote down",
            )
            assertEquals(listOf(31L, 32L), serverTaskIds())
            assertEquals(2, db.tasks().count(), "the history must not show the same run twice")
        }

    @Test
    fun `a run recorded at a different moment is a different run`() =
        runReplicaTest {
            val taskLocalId = seededWithRepeatingTask()
            tickOffOfflineAndSend(taskLocalId)

            enqueueJson(
                changesJson(
                    listOf(
                        upsert(
                            "task",
                            102,
                            32,
                            taskJson(
                                32,
                                "Water the plants",
                                projectId = 11,
                                sourceTaskId = 31,
                                status = "completed",
                                completedAt = WireTime.format(NOW - ONE_DAY),
                            ),
                        ),
                    ),
                    cursor = 200,
                ),
            )
            assertTrue(puller.pull().isApplied)

            // Adopting on anything looser than the exact moment would rewrite a
            // piece of history that belongs to another day.
            assertNotNull(db.tasks().unnamedRecurrenceCompletion(taskLocalId, NOW))
            assertEquals(3, db.tasks().count())
        }

    private companion object {
        /** 2024-03-12T09:00:00.000Z — where the repeating task sits, still ahead of [NOW]. */
        val DUE: Long = WireTime.parse("2024-03-12T09:00:00.000Z")

        val ZONE: ZoneId = ZoneId.of("UTC")

        const val ONE_DAY: Long = 24 * 60 * 60 * 1000L
    }
}
