package ru.tinyops.turboist.core.network.events

import android.util.Log
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.emitAll
import kotlinx.coroutines.flow.flow
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.sse.EventSource
import okhttp3.sse.EventSourceListener
import okhttp3.sse.EventSources
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.api.EventsApi
import java.io.IOException
import java.time.Duration

/** What the change stream tells its collector. */
sealed interface StreamSignal {
    /**
     * The stream is up.
     *
     * @property afterGap true when there was a stretch with no stream at all
     *   before this one opened. Changes made during such a stretch arrive as
     *   nothing at all, so the gap itself is the reason to go and ask; an opening
     *   that follows no gap is the first of a session and whoever started the
     *   stream has already asked.
     */
    data class Opened(val afterGap: Boolean) : StreamSignal

    /**
     * Something changed on the server. The name of the changed area is carried
     * for the log only: this client reacts to every one of them identically, by
     * asking for everything that changed since its stored position.
     */
    data class Changed(val area: String) : StreamSignal

    /** The stream is down. A new one will be opened after a backoff. */
    data class Closed(val cause: Throwable?) : StreamSignal
}

/**
 * The server's change stream, seen by whoever reacts to it.
 *
 * An interface because reacting to changes and receiving them are two different
 * concerns with two different failure modes, and because what the reacting side
 * does is worth testing without a server on the other end.
 */
fun interface ChangeStream {
    /** Signals until the collector stops. Never completes on its own. */
    fun signals(): Flow<StreamSignal>
}

/**
 * The server's change stream, as a flow that never ends.
 *
 * Collect it while the app is in front of the user and cancel the collection
 * when it is not. It handshakes, opens the stream, reports what arrives, and
 * when the stream dies it opens another one after a growing delay — so a
 * collector never has to think about reconnecting, only about what a change
 * means.
 *
 * Two properties of the server shape everything here:
 *
 * - **A ticket is single-use.** So every reconnect re-handshakes for a new one,
 *   and the transport's own retry is never used: replaying the same URL can only
 *   be refused.
 * - **The stream is silent when nothing happens, apart from a heartbeat.** A
 *   socket that dies quietly — a phone whose radio slept, a proxy that dropped an
 *   idle connection — produces no error on its own. The read timeout below is set
 *   above the heartbeat interval and below anything a user would call "stale", so
 *   silence for longer than a couple of heartbeats is what fails the connection
 *   and starts a new one.
 */
class EventStream(
    private val api: EventsApi,
    private val serverUrl: ServerUrl,
    httpClient: OkHttpClient,
    private val backoff: List<Duration> = RECONNECT_BACKOFF,
    private val elapsedMillis: () -> Long = { System.nanoTime() / 1_000_000 },
) : ChangeStream {
    private val factory: EventSource.Factory =
        EventSources.createFactory(
            httpClient.newBuilder()
                // The whole stack's timeout is meant for a request that answers;
                // this one is meant for a connection that mostly says nothing.
                .readTimeout(SILENCE_TIMEOUT)
                .build(),
        )

    /**
     * When the stream currently being collected opened, as an elapsed
     * millisecond reading, or `0` while none has. Written and read from the one
     * coroutine that runs [signals].
     */
    private var openedAt = 0L

    /**
     * Opens the stream and keeps it open for as long as the collector lasts.
     *
     * Signals are buffered, so a collector that takes its time delays the
     * reaction to a change rather than losing it. The flow completes only by
     * cancellation.
     */
    override fun signals(): Flow<StreamSignal> =
        flow {
            var attempt = 0
            var afterGap = false
            while (true) {
                val ticket =
                    try {
                        api.issueTicket().ticket
                    } catch (cancelled: CancellationException) {
                        throw cancelled
                    } catch (unreachable: IOException) {
                        Log.i(LOG_TAG, "Could not get permission to open the change stream", unreachable)
                        emit(StreamSignal.Closed(unreachable))
                        afterGap = true
                        delay(delayFor(attempt++))
                        continue
                    }
                val url = streamUrl(ticket)
                if (url == null) {
                    // No server address means no stream, and no address is a state
                    // the sign-in flow gets out of, not this loop.
                    Log.w(LOG_TAG, "No server address is configured; not opening the change stream")
                    emit(StreamSignal.Closed(null))
                    afterGap = true
                    delay(delayFor(attempt++))
                    continue
                }
                emitAll(connection(url, afterGap))
                val lasted = if (openedAt == 0L) 0L else elapsedMillis() - openedAt
                afterGap = true
                attempt = if (openedAt != 0L && lasted >= HEALTHY_AFTER.toMillis()) 0 else attempt + 1
                delay(delayFor(attempt))
            }
        }

    /**
     * One connection, from opening to whatever ends it.
     *
     * The flow completes when the stream does — cleanly or on a failure — which
     * is what returns control to the retry loop above.
     */
    private fun connection(
        url: HttpUrl,
        afterGap: Boolean,
    ): Flow<StreamSignal> =
        callbackFlow {
            openedAt = 0L
            val listener =
                object : EventSourceListener() {
                    override fun onOpen(
                        eventSource: EventSource,
                        response: Response,
                    ) {
                        openedAt = elapsedMillis()
                        Log.i(LOG_TAG, "The change stream is open")
                        trySend(StreamSignal.Opened(afterGap))
                    }

                    override fun onEvent(
                        eventSource: EventSource,
                        id: String?,
                        type: String?,
                        data: String,
                    ) {
                        // The heartbeat proves the connection is alive and means
                        // nothing changed, so it must not send anyone asking.
                        if (type == null || type == HEARTBEAT_EVENT) return
                        trySend(StreamSignal.Changed(type))
                    }

                    override fun onClosed(eventSource: EventSource) {
                        trySend(StreamSignal.Closed(null))
                        close()
                    }

                    override fun onFailure(
                        eventSource: EventSource,
                        t: Throwable?,
                        response: Response?,
                    ) {
                        Log.i(LOG_TAG, "The change stream ended; a new one will be opened shortly", t)
                        trySend(StreamSignal.Closed(t))
                        close()
                    }
                }
            val source = factory.newEventSource(Request.Builder().url(url).build(), listener)
            awaitClose { source.cancel() }
        }

    private fun streamUrl(ticket: String): HttpUrl? =
        serverUrl.value
            ?.newBuilder()
            ?.addPathSegments(STREAM_PATH)
            ?.addQueryParameter("ticket", ticket)
            ?.build()

    private fun delayFor(attempt: Int): Duration = backoff[attempt.coerceIn(0, backoff.size - 1)]

    private suspend fun delay(duration: Duration) = delay(duration.toMillis())

    companion object {
        private const val LOG_TAG = "TurboistEvents"

        private const val STREAM_PATH = "api/v1/events"

        /** The server's proof-of-life event. Carries no change. */
        private const val HEARTBEAT_EVENT = "ping"

        /**
         * How long the stream may say nothing at all before it counts as dead.
         *
         * The server sends a heartbeat every 25 seconds, so this leaves room for
         * two of them to be late — a phone's radio waking up is enough to delay one
         * — while still noticing a connection that quietly stopped existing within
         * about a minute.
         */
        private val SILENCE_TIMEOUT: Duration = Duration.ofSeconds(70)

        /**
         * A stream that lasted this long counts as having worked, so the next drop
         * starts the delays over from the shortest one.
         *
         * Without the threshold, a server that accepts the stream and drops it a
         * second later would be reconnected once per second forever, and each
         * reconnect drags a catch-up request behind it.
         */
        private val HEALTHY_AFTER: Duration = Duration.ofSeconds(10)

        /**
         * How long to wait before opening the stream again, by consecutive failure.
         * The last entry repeats: a server that has been down for an hour is asked
         * about twice a minute, not once an hour.
         */
        val RECONNECT_BACKOFF: List<Duration> =
            listOf(
                Duration.ofSeconds(1),
                Duration.ofSeconds(2),
                Duration.ofSeconds(4),
                Duration.ofSeconds(8),
                Duration.ofSeconds(15),
                Duration.ofSeconds(30),
            )
    }
}
