package ru.tinyops.turboist.core.database

import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.database.entity.ProjectLabelRow
import ru.tinyops.turboist.core.database.entity.TaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.model.TaskStatus
import kotlin.test.assertEquals

/**
 * The quick switcher's search: tasks and projects found by title, then by a
 * label they carry, then by their description — the same order the server's
 * search answers in, so the two clients agree on what comes first.
 */
class SwitcherSearchTest : ReplicaTest() {
    private suspend fun taskHits(typed: String): List<Pair<String, Int>> {
        val hits =
            db.search().switcherTasks(
                query = requireNotNull(FtsQuery.match(typed)),
                titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
                labelQuery = requireNotNull(FtsQuery.match(typed)),
                limit = 50,
            )
        val titles = db.search().tasksByLocalIds(hits.map { it.localId }).associate { it.localId to it.title }
        return hits.map { titles.getValue(it.localId) to it.hitRank }
    }

    private suspend fun projectHits(typed: String): List<Pair<String, Int>> {
        val hits =
            db.search().switcherProjects(
                query = requireNotNull(FtsQuery.match(typed)),
                titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, typed)),
                labelQuery = requireNotNull(FtsQuery.match(typed)),
                limit = 50,
            )
        val titles = db.search().projectsByLocalIds(hits.map { it.localId }).associate { it.localId to it.title }
        return hits.map { titles.getValue(it.localId) to it.hitRank }
    }

    private suspend fun givenTask(
        title: String,
        description: String = "",
        status: TaskStatus = TaskStatus.OPEN,
    ): Long =
        db.tasks().insert(
            TaskRow(title = title, description = description, status = status, createdAt = NOW, updatedAt = NOW),
        )

    @Test
    fun `tasks come back title hits first, then label hits, then description hits`() =
        runTest {
            val byDescription = givenTask("Позвонить в банк", description = "про отпуск")
            val byLabel = givenTask("Купить билеты")
            givenTask("Отпуск в горах")
            givenTask("Unrelated")
            val label = db.labels().insert(label(name = "отпуск"))
            db.tasks().insertLabels(listOf(TaskLabelRow(taskLocalId = byLabel, labelLocalId = label)))

            assertEquals(
                listOf("Отпуск в горах" to 0, "Купить билеты" to 1, "Позвонить в банк" to 2),
                taskHits("ОТП"),
            )
            check(byDescription > 0)
        }

    @Test
    fun `open work comes before finished work within one rank`() =
        runTest {
            givenTask("Renew passport, old", status = TaskStatus.COMPLETED)
            givenTask("Renew passport, new")

            assertEquals(listOf("Renew passport, new", "Renew passport, old"), taskHits("passport").map { it.first })
        }

    @Test
    fun `projects are found by title, then by a label they carry`() =
        runTest {
            val contextLocalId = db.contexts().insert(context())
            val tagged = db.projects().insert(project(contextLocalId, title = "Dacha"))
            db.projects().insert(project(contextLocalId, title = "Огород"))
            db.projects().insert(project(contextLocalId, title = "Website"))
            val label = db.labels().insert(label(name = "огород"))
            db.projects().insertLabels(listOf(ProjectLabelRow(projectLocalId = tagged, labelLocalId = label)))

            assertEquals(listOf("Огород" to 0, "Dacha" to 1), projectHits("огор"))
        }
}
