package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.blockerServerIds
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The queue of writes that have not reached the server, and the two questions
 * asked of it: what goes next, and which rows must a pull leave alone.
 */
class OutboxTest : ReplicaTest() {
    @Test
    fun `writes queued in the same instant still have an order`() =
        runTest {
            // A bulk action queues several ops at once. Ordering them by the
            // moment they were made would leave the order of these three
            // undefined, and the order is exactly what must survive.
            val first = db.outbox().enqueue(op("a", entityLocalId = 1))
            val second = db.outbox().enqueue(op("b", entityLocalId = 2))
            val third = db.outbox().enqueue(op("c", entityLocalId = 3))

            assertEquals(listOf("a", "b", "c"), db.outbox().all().map { it.id })
            assertTrue(first.seq < second.seq && second.seq < third.seq)
            assertEquals("a", assertNotNull(db.outbox().head()).id)
        }

    @Test
    fun `the head of the queue does not change while it is in flight`() =
        runTest {
            val head = db.outbox().enqueue(op("a", entityLocalId = 1))
            db.outbox().enqueue(op("b", entityLocalId = 2))

            db.outbox().update(head.copy(state = OutboxState.INFLIGHT, attempts = 1))

            val stillHead = assertNotNull(db.outbox().head())
            assertEquals("a", stillHead.id, "nothing may overtake the op being sent")
            assertEquals(OutboxState.INFLIGHT, stillHead.state)
        }

    @Test
    fun `a row with a queued write is dirty`() =
        runTest {
            val taskLocalId = db.tasks().insert(task(title = "Edited offline"))
            db.outbox().enqueue(op("a", entity = ReplicaEntityKind.TASK, entityLocalId = taskLocalId))

            assertTrue(db.outbox().isDirty(ReplicaEntityKind.TASK, taskLocalId))
            assertFalse(
                db.outbox().isDirty(ReplicaEntityKind.PROJECT, taskLocalId),
                "the same number means a different row in a different table",
            )
            assertFalse(db.outbox().isDirty(ReplicaEntityKind.TASK, taskLocalId + 1))
        }

    @Test
    fun `a row stays dirty while its write is in flight or waiting to be retried`() =
        runTest {
            val queued = db.outbox().enqueue(op("a", entityLocalId = 7))

            for (state in OutboxState.entries) {
                db.outbox().update(queued.copy(state = state))
                assertTrue(
                    db.outbox().isDirty(ReplicaEntityKind.TASK, 7),
                    "a write the server has not accepted yet still owns the row, whatever it is called",
                )
            }
        }

    @Test
    fun `a row is clean again once its write has left the queue`() =
        runTest {
            val queued = db.outbox().enqueue(op("a", entityLocalId = 7))
            db.outbox().deleteById(queued.id)

            assertFalse(db.outbox().isDirty(ReplicaEntityKind.TASK, 7))
        }

    @Test
    fun `a whole page of incoming records is checked in one question`() =
        runTest {
            db.outbox().enqueue(op("a", entityLocalId = 2))
            db.outbox().enqueue(op("b", entityLocalId = 4))
            db.outbox().enqueue(op("c", entity = ReplicaEntityKind.PROJECT, entityLocalId = 5))

            assertEquals(
                setOf(2L, 4L),
                db.outbox().dirtyLocalIds(ReplicaEntityKind.TASK, listOf(1, 2, 3, 4, 5)),
            )
            assertTrue(db.outbox().dirtyLocalIds(ReplicaEntityKind.TASK, emptyList()).isEmpty())
        }

    @Test
    fun `a refused write leaves the queue and is kept where the user can see it`() =
        runTest {
            val refused = db.outbox().enqueue(op("a", entityLocalId = 1))
            val behind = db.outbox().enqueue(op("b", entityLocalId = 2))

            db.outbox().quarantine(
                op = refused.copy(attempts = 2),
                errorCode = "task_blocked",
                errorMessage = "a task with an open blocker cannot be completed",
                httpStatus = 409,
                quarantinedAt = NOW + 5,
                blockedBy = listOf(9, 12),
            )

            assertNull(db.outbox().byId("a"), "a refusal is an answer, not something to retry")
            assertEquals(behind.id, assertNotNull(db.outbox().head()).id, "the queue moves on")

            val kept = db.outbox().quarantined().single()
            assertEquals("a", kept.id)
            assertEquals("task_blocked", kept.errorCode)
            assertEquals(409, kept.httpStatus)
            assertEquals(2, kept.attempts)
            assertEquals(refused.payload, kept.payload, "the user is entitled to see what was lost")
            assertEquals(
                listOf(9L, 12L),
                kept.blockerServerIds,
                "the message says the same thing for every blocked task; the ids say which ones",
            )
            assertEquals(ReplicaEntityKind.TASK, kept.entity)
            assertEquals(refused.createdAt, kept.createdAt)

            assertFalse(
                db.outbox().isDirty(ReplicaEntityKind.TASK, 1),
                "a quarantined write no longer protects the row: the next pull is the correction",
            )
        }

    @Test
    fun `a discarded write leaves nothing behind`() =
        runTest {
            val refused = db.outbox().enqueue(op("a", entityLocalId = 1))
            db.outbox().quarantine(refused, "task_blocked", "", 409, NOW)

            assertEquals(1, db.outbox().discardQuarantined("a"))
            assertTrue(db.outbox().quarantined().isEmpty())
        }

    private fun op(
        id: String,
        entity: ReplicaEntityKind = ReplicaEntityKind.TASK,
        entityLocalId: Long,
    ) = OutboxOpRow(
        id = id,
        op = "task.complete",
        payload = """{"id":$entityLocalId}""",
        entity = entity,
        entityLocalId = entityLocalId,
        createdAt = NOW,
        updatedAt = NOW,
    )
}
