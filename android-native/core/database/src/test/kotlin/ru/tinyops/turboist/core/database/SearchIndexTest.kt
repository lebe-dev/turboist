package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertTrue

/**
 * The full-text index is maintained by the schema, not by the code that writes.
 *
 * That is the property worth a test: a row written, edited or deleted by *any*
 * path — a screen, an applied change, a cascade nobody called — must be findable
 * or unfindable accordingly, without the writer having remembered anything.
 */
class SearchIndexTest : ReplicaTest() {
    @Test
    fun `a task is findable by its title and by its description`() =
        runTest {
            val localId =
                db.tasks().insert(
                    task(title = "Renew passport").copy(description = "book an appointment at the embassy"),
                )

            assertEquals(listOf(localId), db.searchIndex().matchingTaskLocalIds("passport"))
            assertEquals(listOf(localId), db.searchIndex().matchingTaskLocalIds("embassy"))
            assertTrue(db.searchIndex().matchingTaskLocalIds("groceries").isEmpty())
        }

    @Test
    fun `editing a task moves it in the index without anyone maintaining it`() =
        runTest {
            val localId = db.tasks().insert(task(title = "Renew passport"))
            val stored = assertNotNull(db.tasks().byLocalId(localId))

            db.tasks().update(stored.copy(title = "Renew driving licence"))

            assertTrue(db.searchIndex().matchingTaskLocalIds("passport").isEmpty())
            assertEquals(listOf(localId), db.searchIndex().matchingTaskLocalIds("licence"))
        }

    @Test
    fun `a task removed by a cascade leaves the index too`() =
        runTest {
            val contextLocalId = db.contexts().upsertByServerId(context(serverId = 1))
            val projectLocalId = db.projects().upsertByServerId(project(contextLocalId, serverId = 2))
            db.tasks().insert(
                task(title = "Renew passport", contextLocalId = contextLocalId, projectLocalId = projectLocalId),
            )

            // Nothing here touches the task table directly.
            db.contexts().deleteByServerId(1)

            assertTrue(db.searchIndex().matchingTaskLocalIds("passport").isEmpty())
        }

    @Test
    fun `projects, labels and contexts are searchable as well`() =
        runTest {
            val contextLocalId = db.contexts().insert(context(name = "Household"))
            val projectLocalId = db.projects().insert(project(contextLocalId, title = "Kitchen renovation"))
            val labelLocalId = db.labels().insert(label(name = "urgent"))

            assertEquals(listOf(contextLocalId), db.searchIndex().matchingContextLocalIds("household"))
            assertEquals(listOf(projectLocalId), db.searchIndex().matchingProjectLocalIds("renovation"))
            assertEquals(listOf(labelLocalId), db.searchIndex().matchingLabelLocalIds("urgent"))
        }

    @Test
    fun `search ignores case and accents, in both of the product's languages`() =
        runTest {
            val english = db.tasks().insert(task(title = "Café Membership"))
            val russian = db.tasks().insert(task(title = "Купить молоко"))

            assertEquals(listOf(english), db.searchIndex().matchingTaskLocalIds("cafe"))
            assertEquals(listOf(english), db.searchIndex().matchingTaskLocalIds("MEMBERSHIP"))
            assertEquals(listOf(russian), db.searchIndex().matchingTaskLocalIds("молоко"))
        }
}
