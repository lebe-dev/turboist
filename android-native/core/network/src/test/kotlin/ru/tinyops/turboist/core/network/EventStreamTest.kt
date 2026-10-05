package ru.tinyops.turboist.core.network

import kotlinx.coroutines.flow.take
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.runBlocking
import mockwebserver3.MockResponse
import org.junit.After
import org.junit.Test
import ru.tinyops.turboist.core.network.events.EventStream
import ru.tinyops.turboist.core.network.events.StreamSignal
import java.time.Duration
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The change stream against a real server socket.
 *
 * Everything worth getting wrong here is on the wire: whether the ticket is
 * fetched again for a second connection, whether a heartbeat is mistaken for a
 * change, whether a stream that ended is reopened at all. None of that is
 * visible to a test that stands in for the transport.
 */
class EventStreamTest {
    private val fixture = NetworkFixture()

    /** Fast enough that a reconnect does not make the test wait for the real backoff. */
    private val stream =
        EventStream(
            api = fixture.network.events,
            serverUrl = fixture.serverUrl,
            httpClient = fixture.network.httpClient,
            backoff = listOf(Duration.ofMillis(1)),
        )

    @After
    fun stopServer() {
        fixture.close()
    }

    @Test
    fun `a change on the server reaches the collector, and a heartbeat does not`() =
        runBlocking {
            fixture.server.enqueue(ticketResponse("first-ticket"))
            fixture.server.enqueue(eventStreamResponse(HEARTBEAT_THEN_CHANGE))
            fixture.server.enqueue(ticketResponse("second-ticket"))
            fixture.server.enqueue(eventStreamResponse(""))

            val signals = stream.signals().take(4).toList()

            assertEquals(StreamSignal.Opened(afterGap = false), signals[0])
            assertEquals(StreamSignal.Changed("tasks"), signals[1])
            assertTrue(signals[2] is StreamSignal.Closed, "a stream that ended is reported as closed")
            assertEquals(
                StreamSignal.Opened(afterGap = true),
                signals[3],
                "reopening after a gap is itself the news: what changed during the gap arrived as silence",
            )
        }

    @Test
    fun `every connection takes a ticket of its own`() =
        runBlocking {
            fixture.server.enqueue(ticketResponse("first-ticket"))
            fixture.server.enqueue(eventStreamResponse(""))
            fixture.server.enqueue(ticketResponse("second-ticket"))
            fixture.server.enqueue(eventStreamResponse(""))

            stream.signals().take(3).toList()

            assertEquals("/api/v1/events/ticket", pathOfNextRequest())
            assertEquals(
                "first-ticket",
                ticketOfNextRequest(),
                "the ticket travels in the stream's own address, because that is where the server reads it",
            )
            assertEquals("/api/v1/events/ticket", pathOfNextRequest())
            assertEquals(
                "second-ticket",
                ticketOfNextRequest(),
                "a ticket is spent by the connection it opened, so a reconnect needs one of its own",
            )
        }

    @Test
    fun `a refused handshake is reported and retried`() =
        runBlocking {
            fixture.server.enqueue(errorResponse(500, ApiErrorCodes.INTERNAL_ERROR))
            fixture.server.enqueue(ticketResponse("later-ticket"))
            fixture.server.enqueue(eventStreamResponse(""))

            val signals = stream.signals().take(2).toList()

            assertTrue(signals[0] is StreamSignal.Closed, "a handshake that failed leaves no stream")
            assertEquals(
                StreamSignal.Opened(afterGap = true),
                signals[1],
                "the stretch with no stream at all counts as a gap, whether or not one was ever open",
            )
        }

    private fun pathOfNextRequest(): String? = fixture.server.takeRequest().url?.encodedPath

    private fun ticketOfNextRequest(): String? {
        val url = assertNotNull(fixture.server.takeRequest().url)
        assertEquals("/api/v1/events", url.encodedPath)
        return url.queryParameter("ticket")
    }

    private fun ticketResponse(ticket: String): MockResponse =
        jsonResponse(200, """{"ticket":"$ticket","expiresIn":60}""")

    private fun eventStreamResponse(body: String): MockResponse =
        MockResponse.Builder()
            .code(200)
            .setHeader("Content-Type", "text/event-stream")
            .body(body)
            .build()

    companion object {
        /**
         * Two events, written the way the server writes them: a name, a payload,
         * and a blank line that ends the event. The heartbeat comes first so that
         * a reader which mistook it for a change would be caught reporting one
         * change too many, in the wrong order.
         */
        private const val HEARTBEAT_THEN_CHANGE =
            "event: ping\ndata: {}\n\nevent: tasks\ndata: {}\n\n"
    }
}
