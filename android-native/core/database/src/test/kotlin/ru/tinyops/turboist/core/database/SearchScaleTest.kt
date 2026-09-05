package ru.tinyops.turboist.core.database

import androidx.room.withTransaction
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Search over a workspace the size of a real one.
 *
 * A search is answered while someone is still typing, so the shape of the query
 * matters as much as its result: the match has to come out of the index, the
 * order and the cap have to be the engine's, and nothing may read every row on
 * the device to decide what to show. A workspace of a few thousand tasks is
 * where a query that quietly scans instead of seeking stops being invisible —
 * both in what it costs and, because a scan is capped before it is ordered, in
 * what it answers.
 */
class SearchScaleTest : ReplicaTest() {
    private companion object {
        /** Larger than a well-used workspace, small enough to build in a test. */
        const val TASKS = 3_000
    }

    @Test
    fun `a search over a few thousand tasks still answers with the best matches`() =
        runTest {
            db.withTransaction {
                for (i in 1..TASKS) {
                    db.tasks().insert(
                        TaskRow(
                            title = "Task number $i",
                            description = "notes for item $i",
                            status = if (i % 3 == 0) TaskStatus.COMPLETED else TaskStatus.OPEN,
                            createdAt = NOW,
                            updatedAt = NOW + i,
                        ),
                    )
                }
                // The one row the search is actually looking for, and the one
                // that only mentions it.
                db.tasks().insert(
                    TaskRow(title = "Renew passport", createdAt = NOW, updatedAt = NOW, status = TaskStatus.OPEN),
                )
                db.tasks().insert(
                    TaskRow(
                        title = "Book flights",
                        description = "passport first",
                        createdAt = NOW,
                        updatedAt = NOW + TASKS,
                        status = TaskStatus.OPEN,
                    ),
                )
            }

            val found =
                db.search().tasks(
                    query = requireNotNull(FtsQuery.match("passport")),
                    titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "passport")),
                    status = null,
                    limit = 50,
                ).map { it.title }

            // The title match leads even though the other row was touched later:
            // the ranking is applied across the whole workspace, not across
            // whatever the first page of a scan happened to hold.
            assertEquals(listOf("Renew passport", "Book flights"), found)
        }

    @Test
    fun `a common term answers within its cap rather than with the whole workspace`() =
        runTest {
            db.withTransaction {
                for (i in 1..TASKS) {
                    db.tasks().insert(
                        TaskRow(title = "Task number $i", createdAt = NOW, updatedAt = NOW + i),
                    )
                }
            }

            val found =
                db.search().tasks(
                    query = requireNotNull(FtsQuery.match("task")),
                    titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "task")),
                    status = null,
                    limit = 50,
                )

            assertEquals(50, found.size)
            // Most recently touched first, so the cap keeps the newest work
            // rather than whichever rows the index happened to reach first.
            assertTrue(found.first().updatedAt > found.last().updatedAt)
        }
}
