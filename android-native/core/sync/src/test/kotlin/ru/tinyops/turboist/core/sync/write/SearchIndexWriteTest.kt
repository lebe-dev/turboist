package ru.tinyops.turboist.core.sync.write

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.search.FtsQuery
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * What an optimistic write does to the full-text index.
 *
 * The whole promise of searching on the device is that it describes what the
 * device holds *now* — including the note written a second ago on a train, which
 * no server has been told about. So each case here makes a change the way a
 * screen makes it, with nothing sent anywhere, and then asks the search whether
 * it can see the change. Nothing maintains the index in between: it is derived
 * from the tables and kept current by the schema, and these cases are what holds
 * that claim to account across the write path.
 */
class SearchIndexWriteTest : WriteTest() {
    private suspend fun findTasks(typed: String): List<String> =
        db.search().tasks(
            query = requireNotNull(FtsQuery.match(typed)),
            titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
            status = null,
            limit = 50,
        ).map { it.title }

    @Test
    fun `a task written offline is findable before anything is sent`() =
        runTest {
            tasks.create(TaskDestination.Inbox, NewTask(title = "Renew passport"))

            assertEquals(listOf("Renew passport"), findTasks("passport"))
            assertEquals(1, queue().size)
        }

    @Test
    fun `a task written offline is findable by its notes as well`() =
        runTest {
            tasks.create(
                TaskDestination.Inbox,
                NewTask(title = "Book flights", description = "the embassy closes at four"),
            )

            assertEquals(listOf("Book flights"), findTasks("embassy"))
        }

    @Test
    fun `renaming a task moves it in the index inside the same write`() =
        runTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Renew passport"))

            tasks.patch(created.entityLocalId, TaskEdit(title = "Renew driving licence"))

            assertTrue(findTasks("passport").isEmpty())
            assertEquals(listOf("Renew driving licence"), findTasks("licence"))
        }

    @Test
    fun `a task deleted on the device stops being findable on it`() =
        runTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Renew passport"))

            tasks.delete(created.entityLocalId)

            assertTrue(findTasks("passport").isEmpty())
        }

    @Test
    fun `finishing a task leaves it findable, which is most of why search reaches into it`() =
        runTest {
            val created = tasks.create(TaskDestination.Inbox, NewTask(title = "Renew passport"))

            tasks.complete(created.entityLocalId)

            assertEquals(listOf("Renew passport"), findTasks("passport"))
        }

    @Test
    fun `a project created offline is findable, and so is the context it sits in`() =
        runTest {
            val contextLocalId = givenContext(name = "Household")

            projects.create(contextLocalId, NewProject(title = "Kitchen renovation"))

            assertEquals(
                listOf("Kitchen renovation"),
                db.search().projects(
                    requireNotNull(FtsQuery.match("renov")),
                    requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "renov")),
                    limit = 50,
                ).map { it.title },
            )
            assertEquals(
                listOf("Household"),
                db.search().contexts(requireNotNull(FtsQuery.match("household")), limit = 50).map { it.name },
            )
        }
}
