package ru.tinyops.turboist.core.network

import kotlinx.coroutines.runBlocking
import ru.tinyops.turboist.core.network.api.requireBody
import ru.tinyops.turboist.core.network.dto.CreateTaskRequest
import ru.tinyops.turboist.core.network.http.isIdempotentReplay
import java.util.concurrent.atomic.AtomicInteger
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * A write whose response is lost must be safe to send again. The key is what makes
 * that true, so these tests pin when it is attached, when it is left alone, and how
 * the server's "I did not run this again" answer reaches the caller.
 */
class IdempotencyTest {
    private val taskJson = """{"id":42,"title":"write it down","status":"open"}"""

    @Test
    fun `every mutation of the versioned API goes out with a key`() {
        NetworkFixture(newIdempotencyKey = { "generated-key" }).use { fixture ->
            fixture.server.enqueue(jsonResponse(201, taskJson))

            runBlocking { fixture.network.tasks.createInInbox(CreateTaskRequest(title = "write it down")) }

            val request = fixture.server.takeRequest()
            assertEquals("generated-key", request.headers[ApiHeaders.IDEMPOTENCY_KEY])
        }
    }

    @Test
    fun `a caller that owns a key keeps it, because a replay must reuse the stored one`() {
        NetworkFixture(newIdempotencyKey = { "generated-key" }).use { fixture ->
            fixture.server.enqueue(jsonResponse(200, taskJson))

            runBlocking { fixture.network.tasks.complete(42, completeBody(), idempotencyKey = "stored-key") }

            assertEquals("stored-key", fixture.server.takeRequest().headers[ApiHeaders.IDEMPOTENCY_KEY])
        }
    }

    @Test
    fun `a read is not stamped`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, taskJson))

            runBlocking { fixture.network.tasks.get(42) }

            assertNull(fixture.server.takeRequest().headers[ApiHeaders.IDEMPOTENCY_KEY])
        }
    }

    @Test
    fun `signing in is not stamped either - the header only means anything inside the versioned API`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(jsonResponse(200, sessionJson()))

            runBlocking { fixture.network.auth.login(loginBody()) }

            assertNull(fixture.server.takeRequest().headers[ApiHeaders.IDEMPOTENCY_KEY])
        }
    }

    @Test
    fun `a write retried after the token expired keeps its key, so it cannot execute twice`() {
        val keys = AtomicInteger()
        val fixture =
            NetworkFixture(
                tokens = { "stale" },
                refresher = { "fresh" },
                newIdempotencyKey = { "generated-key-${keys.incrementAndGet()}" },
            )

        fixture.use {
            fixture.server.enqueue(errorResponse(401, ApiErrorCodes.AUTH_EXPIRED, "access token expired"))
            fixture.server.enqueue(jsonResponse(200, taskJson))

            runBlocking { fixture.network.tasks.complete(42, completeBody()) }

            val first = fixture.server.takeRequest()
            val retry = fixture.server.takeRequest()
            assertEquals("generated-key-1", first.headers[ApiHeaders.IDEMPOTENCY_KEY])
            assertEquals("generated-key-1", retry.headers[ApiHeaders.IDEMPOTENCY_KEY])
            assertEquals(1, keys.get())
        }
    }

    @Test
    fun `a replayed answer is visible to the caller that replayed the write`() {
        NetworkFixture().use { fixture ->
            fixture.server.enqueue(
                jsonResponse(200, taskJson, mapOf(ApiHeaders.IDEMPOTENT_REPLAY to "true")),
            )
            fixture.server.enqueue(jsonResponse(200, taskJson))

            val replayed =
                runBlocking { fixture.network.tasks.complete(42, completeBody(), idempotencyKey = "stored-key") }
            assertTrue(replayed.isIdempotentReplay)
            assertEquals(42L, replayed.requireBody().id)

            val executed =
                runBlocking { fixture.network.tasks.complete(42, completeBody(), idempotencyKey = "other-key") }
            assertFalse(executed.isIdempotentReplay)
        }
    }
}
