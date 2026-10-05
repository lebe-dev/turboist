package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import kotlin.system.measureNanoTime
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * What one catch-up costs in round trips and in bytes.
 *
 * On a phone the expensive part of syncing is not the rows — it is the request.
 * Each one wakes the radio, and a radio that has been woken stays awake and
 * drawing current for far longer than the transfer itself lasts. So the property
 * worth pinning is the *count*: one turn of the engine asks the server once when
 * there is little to collect, and asks it as few times as the contract allows
 * when there is a lot. A change that quietly turns one catch-up into several is
 * invisible in every other test and is felt as battery.
 *
 * The measured sizes are printed rather than asserted. A payload is the server's
 * to decide and grows with a workspace; what this checks is that the device is
 * not asking for it more often than it has to.
 */
class SyncCostTest : SyncTest() {
    /**
     * How many of the server's records this test has already read.
     *
     * The server hands them out one at a time and waits when there are none
     * left, so a case that asks for one more than was made never fails — it
     * hangs. Counting what has been taken is what keeps every case asking for
     * exactly the requests it caused.
     */
    private var taken = 0

    /** The next request the device made, in the order it made them. */
    private fun takeNext() = assertNotNull(server.takeRequest()).also { taken++ }

    /** The requests made since the last one that was read. */
    private fun unreadRequests(): Int = server.requestCount - taken

    @Test
    fun `seeding a replica from nothing costs one request`() =
        runReplicaTest {
            val snapshot = workspaceSnapshot(tasks = 2_000, projects = 200)
            enqueueJson(snapshot)

            val elapsed = measureNanoTime { assertTrue(puller.pull().isApplied) }

            assertEquals(1, unreadRequests(), "a complete copy is one call, whatever it holds")
            assertEquals("/api/v1/sync/snapshot", takeNext().url.encodedPath)
            report("seeding a replica", requests = 1, bytes = snapshot.length, nanos = elapsed)
        }

    @Test
    fun `an ordinary catch-up costs one request and a payload measured in kilobytes`() =
        runReplicaTest {
            seedSmallReplica()
            val page =
                changesJson(
                    (1..3).map { index ->
                        upsert(
                            "task",
                            100L + index,
                            30L + index,
                            taskJson(30L + index, "Changed $index", projectId = 11),
                        )
                    },
                    cursor = 150,
                )
            enqueueJson(page)

            val elapsed = measureNanoTime { assertTrue(puller.pull().isApplied) }

            assertEquals(1, unreadRequests(), "three changed rows are one page and therefore one call")
            assertTrue(
                page.length < KILOBYTES_PER_ORDINARY_CATCH_UP * 1024,
                "an ordinary catch-up now carries ${page.length} bytes",
            )
            report("an ordinary catch-up", requests = 1, bytes = page.length, nanos = elapsed)
        }

    @Test
    fun `a long absence is collected in the fewest pages the server will serve`() =
        runReplicaTest {
            seedSmallReplica()
            val changes = 1_200
            var served = 0
            var bytes = 0
            respondBy { request ->
                val asked = request.url.queryParameter("limit")?.toInt() ?: 0
                val remaining = changes - served
                val page = minOf(asked, remaining)
                val body =
                    changesJson(
                        (1..page).map { index ->
                            val id = 1_000L + served + index
                            upsert("task", id, id, taskJson(id, "Change $id", projectId = 11))
                        },
                        cursor = 200L + served + page,
                        hasMore = served + page < changes,
                    )
                served += page
                bytes += body.length
                jsonResponse(body)
            }

            val elapsed = measureNanoTime { assertTrue(puller.pull().isApplied) }

            // Three pages for twelve hundred changes: the device asks for the
            // largest page the server serves, so a week away costs the fewest
            // wake-ups the contract allows rather than one per handful of rows.
            assertEquals(3, unreadRequests())
            assertEquals(changes, served)
            val pages = unreadRequests()
            for (index in 1..pages) {
                assertEquals(
                    SERVER_PAGE_LIMIT.toString(),
                    takeNext().url.queryParameter("limit"),
                    "page $index asked for a smaller page than the server would have served",
                )
            }
            report("a long absence", requests = 3, bytes = bytes, nanos = elapsed)
        }

    @Test
    fun `a catch-up talks to the change feed and to nothing else`() =
        runReplicaTest {
            seedSmallReplica()
            enqueueJson(
                changesJson(listOf(upsert("task", 101, 31, taskJson(31, "Only", projectId = 11))), cursor = 150),
            )

            assertTrue(puller.pull().isApplied)

            // The replica is the app's read model, so a catch-up has no reason to
            // call an entity endpoint or a summary of anything: everything a
            // screen renders comes out of the rows this wrote.
            val paths = (1..unreadRequests()).map { takeNext().url.encodedPath }
            assertTrue(
                paths.all { it.startsWith("/api/v1/sync/") },
                "a catch-up called something other than the change feed: $paths",
            )
        }

    // --- fixtures ---

    /** A replica that already holds a context and a project, at position 100. */
    private suspend fun seedSmallReplica() {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7)),
                projects = listOf(projectJson(11, contextId = 7)),
            ),
        )
        assertTrue(puller.pull().isApplied)
        takeNext()
    }

    /** A complete copy of a workspace of the stated size, in the shape the API serves. */
    private fun workspaceSnapshot(
        tasks: Int,
        projects: Int,
    ): String =
        snapshotJson(
            cursor = 100,
            contexts = listOf(contextJson(7)),
            projects = (1..projects).map { projectJson(100L + it, contextId = 7, title = "Project $it") },
            tasks =
                (1..tasks).map { index ->
                    taskJson(1_000L + index, "Task $index", projectId = 100L + (index % projects) + 1)
                },
        )

    private fun report(
        what: String,
        requests: Int,
        bytes: Int,
        nanos: Long,
    ) {
        println(
            String.format(
                "%-22s %2d request(s)  %8.1f KB  applied in %7.1f ms",
                what,
                requests,
                bytes / 1024.0,
                nanos / 1_000_000.0,
            ),
        )
    }

    private companion object {
        /**
         * The largest page the server serves, and therefore the one the device
         * asks for. Asking for less would only mean more round trips for the same
         * rows.
         */
        const val SERVER_PAGE_LIMIT = 500

        /**
         * What a catch-up after a few minutes away should stay under. Not a
         * server budget — the server sends what changed — but a guard on this
         * side against a change that made an idle app collect far more than the
         * handful of rows that actually moved.
         */
        const val KILOBYTES_PER_ORDINARY_CATCH_UP = 4
    }
}
