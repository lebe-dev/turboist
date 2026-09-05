package ru.tinyops.turboist.core.sync.pull

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withTimeoutOrNull
import kotlinx.coroutines.yield
import org.junit.Test
import ru.tinyops.turboist.core.sync.SyncMutex
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * How a catch-up behaves when it is asked for from several places at once.
 *
 * It is asked for constantly — a screen refreshing, a background job waking, an
 * event arriving from the server — and those arrive in bursts. Answering each one
 * with its own round trip would spend a phone's radio on questions that were
 * already asked.
 *
 * The rule that decides is about *order*, and it is the strict one: a catch-up
 * answers a request only if it began after that request was made. Everything
 * waiting when it starts shares its one answer; anything asked once it is
 * already running is a younger question that this catch-up cannot have looked
 * for, and gets a round trip of its own. Loosening that to "share with whatever
 * is running" would let a change made during the catch-up go unnoticed until
 * something else happened to ask again.
 */
class SyncCycleTest : SyncTest() {
    @Test
    fun `requests that a running catch-up already covers share its answer`() =
        runTest {
            enqueueJson(snapshotJson(cursor = 100, contexts = listOf(contextJson(7))))
            enqueueJson(changesJson(emptyList(), cursor = 100))

            val results = listOf(async { puller.pull() }, async { puller.pull() }, async { puller.pull() }).awaitAll()

            assertTrue(results.all { it.isApplied })
            assertEquals(
                2,
                server.requestCount,
                "the first request is served by the copy, and the two behind it share one catch-up",
            )
        }

    @Test
    fun `a request made while a catch-up is in flight gets one of its own`() =
        runReplicaTest {
            val reachedServer = CountDownLatch(1)
            val letItAnswer = CountDownLatch(1)
            respondBy { request ->
                if (!request.url.encodedPath.endsWith("/sync/snapshot")) {
                    jsonResponse(changesJson(emptyList(), cursor = 100))
                } else {
                    reachedServer.countDown()
                    letItAnswer.await()
                    jsonResponse(snapshotJson(cursor = 100, contexts = listOf(contextJson(7))))
                }
            }

            val running = async(Dispatchers.Default) { puller.pull() }
            assertTrue(reachedServer.await(5, TimeUnit.SECONDS), "the first catch-up should have reached the server")

            // Asked for only now, with the first catch-up already on the wire:
            // whatever it comes back with was looked up before this question
            // existed, so it cannot stand as the answer to it.
            val asked = async(Dispatchers.Default) { puller.pull() }
            letItAnswer.countDown()

            assertTrue(running.await().isApplied)
            assertTrue(asked.await().isApplied)
            assertEquals(
                2,
                server.requestCount,
                "a question younger than the catch-up running when it was asked needs its own round trip",
            )
        }

    @Test
    fun `a request made after a catch-up finished gets one of its own`() =
        runReplicaTest {
            enqueueJson(snapshotJson(cursor = 100, contexts = listOf(contextJson(7))))
            assertTrue(puller.pull().isApplied)

            enqueueJson(changesJson(emptyList(), cursor = 100))
            assertTrue(puller.pull().isApplied)

            assertEquals(2, server.requestCount, "a question asked after the last answer is a new question")
        }

    @Test
    fun `a catch-up waits while the other half of the cycle holds the lock`() =
        runReplicaTest {
            val lock = SyncMutex()
            val gate = CompletableDeferred<Unit>()
            val sending = launch(Dispatchers.Default) { lock.withExclusiveAccess { gate.await() } }
            while (!lock.isHeld) yield()

            enqueueJson(snapshotJson(cursor = 100, contexts = listOf(contextJson(7))))
            val reading =
                async(Dispatchers.Default) {
                    SyncPuller(network.sync, db, applier, lock).pull()
                }

            assertNull(
                withTimeoutOrNull(200) { reading.await() },
                "reading the server while a write is in flight can overwrite the row that write is about",
            )
            assertEquals(0, server.requestCount)

            gate.complete(Unit)
            sending.join()

            assertTrue(reading.await().isApplied)
        }
}
