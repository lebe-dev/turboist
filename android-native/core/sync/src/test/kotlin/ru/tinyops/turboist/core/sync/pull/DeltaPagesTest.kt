package ru.tinyops.turboist.core.sync.pull

import mockwebserver3.RecordedRequest
import org.junit.Test
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.SyncChangesDto
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Catching up page by page, and surviving being interrupted while doing it. */
class DeltaPagesTest : SyncTest() {
    /** A replica that already holds a context and a project, at position 100. */
    private suspend fun seeded() {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7)),
                projects = listOf(projectJson(11, contextId = 7)),
            ),
        )
        assertTrue(puller.pull().isApplied)
        server.takeRequest()
    }

    private fun nextRequest(): RecordedRequest = assertNotNull(server.takeRequest())

    @Test
    fun `pages are asked for from the position the last one left behind`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "First", projectId = 11))),
                    cursor = 150,
                    hasMore = true,
                ),
            )
            enqueueJson(
                changesJson(listOf(upsert("task", 151, 32, taskJson(32, "Second", projectId = 11))), cursor = 200),
            )

            val result = puller.pull()

            assertEquals(PullResult.Applied(epoch = 1, cursor = 200, changes = 2, fromSnapshot = false), result)
            assertEquals(listOf(31L, 32L), serverTaskIds())
            assertEquals("100", nextRequest().url.queryParameter("since"))
            assertEquals("150", nextRequest().url.queryParameter("since"))
            assertEquals(200L, assertNotNull(db.syncState().get()).cursor)
        }

    @Test
    fun `a process that dies between pages resumes from the page that landed`() =
        runReplicaTest {
            seeded()

            // The first page is applied and the process goes away before the
            // second one is even asked for — the state a crash leaves behind.
            val landed =
                applier.applyPage(
                    TurboistJson.decodeFromString(
                        SyncChangesDto.serializer(),
                        changesJson(
                            listOf(upsert("task", 101, 31, taskJson(31, "First", projectId = 11))),
                            cursor = 150,
                            hasMore = true,
                        ),
                    ),
                )

            assertTrue(landed is PageOutcome.Applied)
            assertEquals(
                150L,
                assertNotNull(db.syncState().get()).cursor,
                "the page and its position committed together",
            )
            assertEquals(listOf(31L), serverTaskIds())

            // What starts up next asks from exactly where that page left off. The
            // server answers nothing else, so resuming from any other position
            // fails the case rather than being quietly satisfied.
            respondBy { request ->
                if (request.url.queryParameter("since") == "150") {
                    jsonResponse(
                        changesJson(
                            listOf(upsert("task", 151, 32, taskJson(32, "Second", projectId = 11))),
                            cursor = 200,
                        ),
                    )
                } else {
                    jsonResponse("""{"error":{"code":"conflict","message":"asked from the wrong position"}}""", 409)
                }
            }

            assertTrue(newPuller().pull().isApplied)

            assertEquals(listOf(31L, 32L), serverTaskIds())
            assertEquals(200L, assertNotNull(db.syncState().get()).cursor)
        }

    @Test
    fun `a page delivered twice leaves the same replica behind`() =
        runReplicaTest {
            seeded()
            val page = changesJson(listOf(upsert("task", 101, 31, taskJson(31, "Once", projectId = 11))), cursor = 150)
            enqueueJson(page)
            assertTrue(puller.pull().isApplied)
            val localId = assertNotNull(db.tasks().byServerId(31)).localId

            enqueueJson(page)
            assertTrue(puller.pull().isApplied)

            assertEquals(listOf(31L), serverTaskIds())
            assertEquals(localId, assertNotNull(db.tasks().byServerId(31)).localId, "the row keeps its device id")
        }

    @Test
    fun `a subtask that arrives ahead of its parent still lands`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(
                        upsert("task", 101, 32, taskJson(32, "Child", projectId = 11, parentId = 31)),
                        upsert("task", 102, 31, taskJson(31, "Parent", projectId = 11)),
                    ),
                    cursor = 150,
                ),
            )

            assertTrue(puller.pull().isApplied)

            val parent = assertNotNull(db.tasks().byServerId(31))
            assertEquals(parent.localId, assertNotNull(db.tasks().byServerId(32)).parentLocalId)
        }

    @Test
    fun `a change naming a row this device does not hold sends it back to a complete copy`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(listOf(upsert("task", 101, 31, taskJson(31, "Elsewhere", projectId = 99))), cursor = 150),
            )
            enqueueJson(
                snapshotJson(
                    cursor = 300,
                    contexts = listOf(contextJson(7)),
                    projects = listOf(projectJson(11, contextId = 7), projectJson(99, contextId = 7, title = "Other")),
                    tasks = listOf(taskJson(31, "Elsewhere", projectId = 99)),
                ),
            )

            val result = puller.pull()

            assertTrue(result is PullResult.Applied && result.fromSnapshot)
            assertEquals(300L, assertNotNull(db.syncState().get()).cursor, "the unplaceable page moved nothing")
            assertEquals("/api/v1/sync/changes", nextRequest().url.encodedPath)
            assertEquals("/api/v1/sync/snapshot", nextRequest().url.encodedPath)
            val elsewhere = assertNotNull(db.projects().byServerId(99))
            assertEquals(elsewhere.localId, assertNotNull(db.tasks().byServerId(31)).projectLocalId)
        }

    @Test
    fun `a deletion takes everything the schema hangs off it`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(
                        upsert("task", 101, 31, taskJson(31, "Parent", projectId = 11)),
                        upsert("task", 102, 32, taskJson(32, "Child", projectId = 11, parentId = 31)),
                    ),
                    cursor = 150,
                ),
            )
            assertTrue(puller.pull().isApplied)
            assertEquals(listOf(31L, 32L), serverTaskIds())

            enqueueJson(changesJson(listOf(tombstone("project", 151, 11)), cursor = 200))

            assertTrue(puller.pull().isApplied)

            assertNull(db.projects().byServerId(11))
            assertEquals(emptyList(), serverTaskIds(), "the tasks in a deleted project go with it")
        }

    @Test
    fun `a record changed and deleted in the same page ends up deleted`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(
                        upsert("task", 101, 31, taskJson(31, "Fleeting", projectId = 11)),
                        tombstone("task", 102, 31),
                    ),
                    cursor = 150,
                ),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals(emptyList(), serverTaskIds())
        }
}
