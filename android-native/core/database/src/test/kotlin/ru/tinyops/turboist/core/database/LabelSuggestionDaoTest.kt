package ru.tinyops.turboist.core.database

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import kotlinx.coroutines.withContext
import org.junit.Test
import ru.tinyops.turboist.core.database.dao.LabelPair
import ru.tinyops.turboist.core.database.entity.TextEmbeddingRow
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** What label suggestions read from the replica, and the cache they keep beside it. */
class LabelSuggestionDaoTest : ReplicaTest() {
    @Test
    fun `the workspace is read with each task's project and every label link`() =
        runTest {
            val projectLocalId = db.projects().insert(project(db.contexts().insert(context()), title = "Soika"))
            val health = db.labels().insert(label(name = "здоровье"))
            val money = db.labels().insert(label(name = "деньги"))
            db.projects().setLabels(projectLocalId, listOf(money))
            val filed = db.tasks().insert(task(title = "Оплатить хостинг", projectLocalId = projectLocalId))
            val loose = db.tasks().insert(task(title = "Записаться к врачу"))
            db.tasks().setLabels(loose, listOf(health), taggedAt = NOW)

            val workspace = db.labelSuggestions().workspace()

            assertEquals(
                mapOf(filed to projectLocalId, loose to null),
                workspace.tasks.associate { it.localId to it.projectLocalId },
            )
            assertEquals(listOf(LabelPair(loose, health)), workspace.taskLabels)
            assertEquals(listOf(LabelPair(projectLocalId, money)), workspace.projectLabels)
            assertEquals(setOf("здоровье", "деньги"), workspace.labels.map { it.name }.toSet())
            assertEquals("Soika", workspace.projects.single().title)
        }

    @Test
    fun `an embedding is stored, replaced and removed by its key`() =
        runTest {
            val dao = db.textEmbeddings()
            dao.putEmbeddings(listOf(TextEmbeddingRow("a", byteArrayOf(1, 2)), TextEmbeddingRow("b", byteArrayOf(3))))
            dao.putEmbeddings(listOf(TextEmbeddingRow("a", byteArrayOf(9))))

            assertContentEquals(byteArrayOf(9), dao.embeddings().single { it.key == "a" }.vector)

            dao.deleteEmbeddings(listOf("a"))

            assertEquals(listOf("b"), dao.embeddingKeys())
        }

    @Test
    fun `wiping the replica empties the embedding cache too`() =
        runTest {
            db.textEmbeddings().putEmbeddings(listOf(TextEmbeddingRow("a", byteArrayOf(1))))

            withContext(Dispatchers.IO) { db.clearAllTables() }

            assertTrue(db.textEmbeddings().embeddingKeys().isEmpty())
        }
}
