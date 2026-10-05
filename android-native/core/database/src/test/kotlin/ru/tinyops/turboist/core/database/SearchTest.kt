package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.database.search.rebuildSearchIndex
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Search over the replica: what it finds, and in what order.
 *
 * Run against a real engine because both halves of the claim are the engine's:
 * the tokenizer decides that `cafe` finds `Café` and that `мол` finds `молоко`,
 * and the ordering is a statement the query planner executes. A stub would agree
 * with whatever was written here, which is exactly what is under test.
 */
class SearchTest : ReplicaTest() {
    private suspend fun findTasks(
        typed: String,
        status: TaskStatus? = null,
        limit: Int = 50,
    ): List<String> =
        db.search().tasks(
            query = requireNotNull(FtsQuery.match(typed)),
            titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
            status = status?.wire,
            limit = limit,
        ).map { it.title }

    private suspend fun givenTask(
        title: String,
        description: String = "",
        status: TaskStatus = TaskStatus.OPEN,
        updatedAt: Long = NOW,
    ): Long =
        db.tasks().insert(
            TaskRow(
                title = title,
                description = description,
                status = status,
                createdAt = NOW,
                updatedAt = updatedAt,
            ),
        )

    // --- what a typed query means -------------------------------------------

    @Test
    fun `a word is matched while it is still being typed`() =
        runTest {
            givenTask("Renew passport")

            assertEquals(listOf("Renew passport"), findTasks("pass"))
            assertEquals(listOf("Renew passport"), findTasks("passport"))
        }

    @Test
    fun `a second word narrows rather than widens`() =
        runTest {
            givenTask("Renew passport")
            givenTask("Renew driving licence")

            assertEquals(listOf("Renew passport"), findTasks("renew pass"))
            assertEquals(emptyList(), findTasks("renew groceries"))
        }

    @Test
    fun `punctuation is separation, never syntax`() =
        runTest {
            givenTask("Ship the c++ rewrite")

            // Each of these would be an operator to the index if it reached it:
            // a quote opens a phrase, a star is a prefix, a minus excludes.
            assertEquals(listOf("Ship the c++ rewrite"), findTasks("\"rewrite"))
            assertEquals(listOf("Ship the c++ rewrite"), findTasks("c++ rewrite"))
            assertEquals(listOf("Ship the c++ rewrite"), findTasks("-rewrite*"))
        }

    @Test
    fun `case and accents are ignored, in both of the product's languages`() =
        runTest {
            givenTask("Café Membership")
            givenTask("Купить молоко")

            assertEquals(listOf("Café Membership"), findTasks("cafe"))
            assertEquals(listOf("Café Membership"), findTasks("MEMBERSHIP"))
            assertEquals(listOf("Купить молоко"), findTasks("МОЛОКО"))
            assertEquals(listOf("Купить молоко"), findTasks("мол"))
            assertEquals(listOf("Купить молоко"), findTasks("купить мол"))
        }

    @Test
    fun `a query too short to be worth running is refused before it reaches the index`() {
        assertEquals(null, FtsQuery.match("a"))
        assertEquals(null, FtsQuery.match("   "))
        assertEquals(null, FtsQuery.match("-,"))
    }

    // --- the order results come back in --------------------------------------

    @Test
    fun `a title match comes before a match in the body`() =
        runTest {
            givenTask("Book flights", description = "renew the passport first")
            givenTask("Renew passport")

            assertEquals(listOf("Renew passport", "Book flights"), findTasks("passport"))
        }

    @Test
    fun `work still open comes before work already finished`() =
        runTest {
            givenTask("Renew passport, old", status = TaskStatus.COMPLETED)
            givenTask("Renew passport, new")

            assertEquals(listOf("Renew passport, new", "Renew passport, old"), findTasks("passport"))
        }

    @Test
    fun `between equal matches the more recently touched comes first`() =
        runTest {
            givenTask("Renew passport, older", updatedAt = NOW - 10_000)
            givenTask("Renew passport, newer", updatedAt = NOW)

            assertEquals(listOf("Renew passport, newer", "Renew passport, older"), findTasks("passport"))
        }

    @Test
    fun `a capped search keeps the best matches rather than an arbitrary slice`() =
        runTest {
            givenTask("Passport photos", updatedAt = NOW - 20_000)
            givenTask("Renew passport", updatedAt = NOW)
            givenTask("Book flights", description = "passport needed", updatedAt = NOW + 10_000)

            assertEquals(listOf("Renew passport", "Passport photos"), findTasks("passport", limit = 2))
        }

    // --- narrowing ------------------------------------------------------------

    @Test
    fun `a status narrows the search to one kind of task`() =
        runTest {
            givenTask("Renew passport")
            givenTask("Renew passport, done", status = TaskStatus.COMPLETED)

            assertEquals(listOf("Renew passport"), findTasks("passport", status = TaskStatus.OPEN))
            assertEquals(listOf("Renew passport, done"), findTasks("passport", status = TaskStatus.COMPLETED))
        }

    // --- the other three kinds ------------------------------------------------

    @Test
    fun `projects, labels and contexts answer the same search`() =
        runTest {
            val contextLocalId = db.contexts().insert(context(name = "Household"))
            db.projects().insert(project(contextLocalId, title = "Kitchen renovation"))
            db.labels().insert(label(name = "renovation-budget"))

            val query = requireNotNull(FtsQuery.match("renov"))
            val titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "renov"))

            assertEquals(
                listOf("Kitchen renovation"),
                db.search().projects(query, titleQuery, limit = 50).map { it.title },
            )
            assertEquals(
                listOf("renovation-budget"),
                db.search().labels(query, limit = 50).map { it.name },
            )
            assertEquals(
                listOf("Household"),
                db.search().contexts(
                    requireNotNull(FtsQuery.match("house")),
                    limit = 50,
                ).map { it.name },
            )
        }

    @Test
    fun `a project is found by its description as well, behind the ones named for it`() =
        runTest {
            val contextLocalId = db.contexts().insert(context())
            db.projects().insert(
                project(contextLocalId, title = "Spring cleaning").copy(description = "the kitchen first"),
            )
            db.projects().insert(project(contextLocalId, title = "Kitchen renovation"))

            val query = requireNotNull(FtsQuery.match("kitchen"))
            val titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "kitchen"))

            assertEquals(
                listOf("Kitchen renovation", "Spring cleaning"),
                db.search().projects(query, titleQuery, limit = 50).map { it.title },
            )
        }

    // --- repair ---------------------------------------------------------------

    @Test
    fun `an index emptied behind the tables' back is restored by a rebuild`() =
        runTest {
            givenTask("Renew passport")
            val contextLocalId = db.contexts().insert(context(name = "Household"))
            db.projects().insert(project(contextLocalId, title = "Kitchen renovation"))
            db.labels().insert(label(name = "urgent"))

            // What a wholesale change to a table looks like from the index: the
            // rows are still there, the entries describing them are not.
            for (index in listOf("tasks_fts", "projects_fts", "labels_fts", "contexts_fts")) {
                db.openHelper.writableDatabase.execSQL("DELETE FROM $index")
            }
            assertTrue(findTasks("passport").isEmpty())

            db.rebuildSearchIndex()

            assertEquals(listOf("Renew passport"), findTasks("passport"))
            assertEquals(
                listOf("Kitchen renovation"),
                db.search().projects(
                    requireNotNull(FtsQuery.match("renov")),
                    requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "renov")),
                    limit = 50,
                ).map { it.title },
            )
            assertEquals(
                listOf("urgent"),
                db.search().labels(requireNotNull(FtsQuery.match("urgent")), limit = 50).map { it.name },
            )
            assertEquals(
                listOf("Household"),
                db.search().contexts(requireNotNull(FtsQuery.match("household")), limit = 50).map { it.name },
            )
        }
}
