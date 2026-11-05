package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import kotlinx.serialization.SerializationException
import mockwebserver3.MockResponse
import mockwebserver3.SocketEffect
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto
import java.io.IOException
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

/**
 * A response read as it arrives must fail exactly the way a response read whole
 * did.
 *
 * The sync engine tells "the network went away" from "this is not what the server
 * promises" by the type of the failure: the first is a stalled queue and a retry,
 * the second is a defect. Reading the body in pieces moves the moment bytes are
 * read into the middle of decoding, which is the one place those two could get
 * confused.
 */
class StreamingJsonTest {
    private fun snapshotBody(taskCount: Int): String {
        val tasks =
            (1..taskCount).joinToString(",") { id ->
                """{"id":$id,"title":"Задача № $id","status":"open","priority":"high",""" +
                    """"createdAt":"2024-03-09T12:34:56.789Z","updatedAt":"2024-03-09T12:34:56.789Z"}"""
            }
        return """{"epoch":3,"cursor":42,"tasks":[$tasks]}"""
    }

    @Test
    fun `a body far larger than any read buffer decodes whole and in order`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, snapshotBody(LARGE_TASK_COUNT)))

            val snapshot = runBlocking { fixture.network.sync.snapshot() }

            assertEquals(42, snapshot.cursor)
            assertEquals((1L..LARGE_TASK_COUNT).toList(), snapshot.tasks.map { it.id })
            assertEquals("Задача № $LARGE_TASK_COUNT", snapshot.tasks.last().title)
        }
    }

    @Test
    fun `a field this build does not know is ignored`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"epoch":3,"cursor":42,"fromTheFuture":{"a":[1,2]}}"""))

            val snapshot: SyncSnapshotDto = runBlocking { fixture.network.sync.snapshot() }

            assertEquals(3, snapshot.epoch)
        }
    }

    @Test
    fun `anything after the document is refused rather than dropped`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, """{"epoch":3,"cursor":42} trailing"""))

            assertFailsWith<SerializationException> {
                runBlocking { fixture.network.sync.snapshot() }
            }
        }
    }

    @Test
    fun `a connection lost part way through a body is a network failure`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                MockResponse.Builder()
                    .code(200)
                    .setHeader("Content-Type", "application/json")
                    .body(snapshotBody(LARGE_TASK_COUNT))
                    .onResponseBody(SocketEffect.ShutdownConnection)
                    .build(),
            )

            assertFailsWith<IOException> {
                runBlocking { fixture.network.sync.snapshot() }
            }
        }
    }

    private companion object {
        const val LARGE_TASK_COUNT: Int = 5_000
    }
}
