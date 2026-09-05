package ru.tinyops.turboist.core.sync.pull

import org.junit.Test
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.SyncSnapshotDto
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * What a catch-up does to the full-text index.
 *
 * Search is answered entirely from the device, so "the index agrees with the
 * replica" is not an internal detail: a change that arrived from another device
 * and never reached the index is a task the user cannot find on this one. The
 * cases below therefore never touch the index directly — they let a pull do what
 * a pull does, and then ask the search whether it can see the result.
 */
class SearchIndexPullTest : SyncTest() {
    private suspend fun findTasks(typed: String): List<String> =
        db.search().tasks(
            query = requireNotNull(FtsQuery.match(typed)),
            titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
            status = null,
            limit = 50,
        ).map { it.title }

    private suspend fun findProjects(typed: String): List<String> =
        db.search().projects(
            query = requireNotNull(FtsQuery.match(typed)),
            titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
            limit = 50,
        ).map { it.title }

    /** A replica seeded from a complete copy, holding one context and one project. */
    private suspend fun seeded() {
        enqueueJson(
            snapshotJson(
                cursor = 100,
                contexts = listOf(contextJson(7, name = "Household")),
                projects = listOf(projectJson(11, contextId = 7, title = "Kitchen renovation")),
            ),
        )
        assertTrue(puller.pull().isApplied)
        server.takeRequest()
    }

    @Test
    fun `a complete copy leaves every kind it carried searchable`() =
        runReplicaTest {
            enqueueJson(
                snapshotJson(
                    contexts = listOf(contextJson(7, name = "Household")),
                    labels = listOf(labelJson(3, name = "urgent")),
                    projects = listOf(projectJson(11, contextId = 7, title = "Kitchen renovation")),
                    tasks = listOf(taskJson(31, "Renew passport", projectId = 11)),
                ),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals(listOf("Renew passport"), findTasks("passport"))
            assertEquals(listOf("Kitchen renovation"), findProjects("renov"))
            assertEquals(
                listOf("urgent"),
                db.search().labels(requireNotNull(FtsQuery.match("urgent")), limit = 50).map { it.name },
            )
            assertEquals(
                listOf("Household"),
                db.search().contexts(requireNotNull(FtsQuery.match("household")), limit = 50).map { it.name },
            )
        }

    @Test
    fun `a complete copy repairs an index the tables had got ahead of`() =
        runReplicaTest {
            seeded()
            // The state a wholesale change to a table leaves behind: rows the
            // index knows nothing about.
            db.openHelper.writableDatabase.execSQL("DELETE FROM projects_fts")
            assertTrue(findProjects("renov").isEmpty())

            // Asked of the applier rather than of the puller: a replica that
            // already has a position asks for changes, and this case is about
            // what a complete copy does when one is served.
            applier.applySnapshot(
                TurboistJson.decodeFromString(
                    SyncSnapshotDto.serializer(),
                    snapshotJson(
                        cursor = 200,
                        contexts = listOf(contextJson(7, name = "Household")),
                        projects = listOf(projectJson(11, contextId = 7, title = "Kitchen renovation")),
                    ),
                ),
            )

            assertEquals(listOf("Kitchen renovation"), findProjects("renov"))
        }

    @Test
    fun `a task that arrives in a page of changes is findable at once`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "Renew passport", projectId = 11))),
                    cursor = 150,
                ),
            )

            assertTrue(puller.pull().isApplied)

            assertEquals(listOf("Renew passport"), findTasks("passport"))
        }

    @Test
    fun `a task renamed on another device moves in the index`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "Renew passport", projectId = 11))),
                    cursor = 150,
                ),
            )
            assertTrue(puller.pull().isApplied)
            server.takeRequest()

            enqueueJson(
                changesJson(
                    listOf(upsert("task", 151, 31, taskJson(31, "Renew driving licence", projectId = 11))),
                    cursor = 200,
                ),
            )
            assertTrue(puller.pull().isApplied)

            assertTrue(findTasks("passport").isEmpty())
            assertEquals(listOf("Renew driving licence"), findTasks("licence"))
        }

    @Test
    fun `a task deleted elsewhere stops being findable here`() =
        runReplicaTest {
            seeded()
            enqueueJson(
                changesJson(
                    listOf(upsert("task", 101, 31, taskJson(31, "Renew passport", projectId = 11))),
                    cursor = 150,
                ),
            )
            assertTrue(puller.pull().isApplied)
            server.takeRequest()

            enqueueJson(changesJson(listOf(tombstone("task", 151, 31)), cursor = 200))
            assertTrue(puller.pull().isApplied)

            assertTrue(findTasks("passport").isEmpty())
        }
}
