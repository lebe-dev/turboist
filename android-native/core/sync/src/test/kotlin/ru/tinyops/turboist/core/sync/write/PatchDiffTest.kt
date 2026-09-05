package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.network.Clearable
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.PatchTaskRequest
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * An edit carries what changed and nothing else.
 *
 * This is what lets two devices edit different fields of the same task and both
 * keep their edit, with no merge rule anywhere: the requests do not overlap. A
 * request that resent every field would instead overwrite whatever moved on the
 * server while it sat in the queue — silently, and in favour of whichever device
 * reconnected last.
 */
class PatchDiffTest : WriteTest() {
    @Test
    fun `an edit sends only the fields it touched`() =
        runTest {
            val taskLocalId = givenTask(title = "Original")

            tasks.patch(taskLocalId, TaskEdit(title = "Renamed"))

            assertEquals(setOf("title"), body().keys)
            assertEquals("Renamed", body()["title"]?.jsonPrimitive?.content)
        }

    @Test
    fun `emptying a field says so out loud`() =
        runTest {
            val taskLocalId = givenTask(title = "Original")

            tasks.patch(taskLocalId, TaskEdit(dueAt = Clearable.Clear))

            // An absent key means "leave it alone", so emptying a date has to be
            // written rather than omitted — the two would be indistinguishable.
            assertEquals(setOf("dueAt"), body().keys)
            assertEquals(JsonNull, body()["dueAt"])
            assertEquals(null, assertNotNull(db.tasks().byLocalId(taskLocalId)).dueAt)
        }

    @Test
    fun `setting a date sends the moment in the form the server reads`() =
        runTest {
            val taskLocalId = givenTask(title = "Original")

            tasks.patch(taskLocalId, TaskEdit(dueAt = Clearable.Set(NOW), dueHasTime = true))

            assertEquals("2024-03-09T12:34:56.789Z", body()["dueAt"]?.jsonPrimitive?.content)
            assertEquals(NOW, assertNotNull(db.tasks().byLocalId(taskLocalId)).dueAt)
        }

    @Test
    fun `an untouched field is neither sent nor changed`() =
        runTest {
            val taskLocalId = givenTask(title = "Original")
            db.tasks().update(
                assertNotNull(db.tasks().byLocalId(taskLocalId)).copy(
                    description = "Some detail",
                    priority = Priority.HIGH,
                ),
            )

            tasks.patch(taskLocalId, TaskEdit(title = "Renamed"))

            val row = assertNotNull(db.tasks().byLocalId(taskLocalId))
            assertEquals("Some detail", row.description)
            assertEquals(Priority.HIGH, row.priority)
            assertTrue("priority" !in body().keys)
            assertTrue("description" !in body().keys)
        }

    @Test
    fun `a created task sends only what was filled in`() =
        runTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Buy milk"))

            val op = queuedOps().single() as CreateTaskOp
            val encoded = Json.parseToJsonElement(OutboxOpCodec.encode(op)).jsonObject
            val body = assertNotNull(encoded["body"]).jsonObject
            assertEquals(setOf("title"), body.keys, "an empty field is not a value the server should store")
        }

    /**
     * The request body the single queued edit becomes.
     *
     * The assertions are made against the body rather than against the stored
     * payload on purpose: the promise is about what reaches the server, and the
     * stored form is free to spell things differently as long as it rebuilds
     * this exactly.
     */
    private suspend fun body(): JsonObject {
        val op = OutboxOpCodec.decode(queue().single().payload) as PatchTaskOp
        return TurboistJson.encodeToJsonElement(PatchTaskRequest.serializer(), op.patch.toRequest()).jsonObject
    }
}
