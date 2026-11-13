package ru.tinyops.turboist.nativeapp.labelsuggest

import org.junit.Test
import ru.tinyops.turboist.core.database.dao.LabelPair
import ru.tinyops.turboist.core.database.dao.SuggestionLabelFacts
import ru.tinyops.turboist.core.database.dao.SuggestionProjectFacts
import ru.tinyops.turboist.core.database.dao.SuggestionTaskFacts
import ru.tinyops.turboist.core.database.dao.SuggestionWorkspace
import ru.tinyops.turboist.core.model.ProjectStatus
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

/** Which text stands for which row, and what makes it into an index. */
class SuggestionIndexPlanTest {
    private val money = 1L
    private val health = 2L
    private val soika = 10L

    private val workspace =
        SuggestionWorkspace(
            tasks =
                listOf(
                    SuggestionTaskFacts(100, "Оплатить хостинг", soika, createdAt = 1),
                    SuggestionTaskFacts(101, "Записаться к врачу", null, createdAt = 3),
                    SuggestionTaskFacts(102, "  ", null, createdAt = 2),
                ),
            taskLabels = listOf(LabelPair(100, money), LabelPair(101, health)),
            labels =
                listOf(
                    SuggestionLabelFacts(money, "деньги", "Всё что связано с финансами."),
                    SuggestionLabelFacts(health, "здоровье", ""),
                ),
            projects = listOf(SuggestionProjectFacts(soika, "Soika", "Альтернатива Sentry")),
            projectLabels = listOf(LabelPair(soika, money)),
        )

    private val plan = SuggestionIndexPlan(workspace)

    @Test
    fun `a label is described with the projects that carry it`() {
        assertEquals(
            "title: деньги | text: Всё что связано с финансами. Projects: Soika (Альтернатива Sentry)",
            plan.labelTexts[money],
        )
        assertEquals("title: здоровье | text: здоровье", plan.labelTexts[health])
        assertEquals("title: Soika | text: Альтернатива Sentry", plan.projectDocuments[soika])
    }

    @Test
    fun `tasks are embedded newest first, and an untitled one not at all`() {
        assertEquals(listOf(101L, 100L), plan.taskTexts.map { it.first })
        assertEquals("task: classification | query: Записаться к врачу", plan.taskTexts.first().second)
    }

    @Test
    fun `labels and projects lead the texts, so the first slice makes an index usable`() {
        assertEquals(plan.labelTexts.values.toSet(), plan.allTexts.take(2).toSet())
        assertEquals(plan.projectDocuments.getValue(soika), plan.allTexts[2])
    }

    @Test
    fun `only what has a vector goes into the index`() {
        val known = plan.allTexts.take(3).toSet() + plan.taskTexts.first().second
        val index = plan.build { text -> if (text in known) floatArrayOf(1f, 0f) else null }

        assertEquals(setOf(money, health), index.labels.map { it.labelId }.toSet())
        assertEquals(listOf(101L), index.history.map { it.taskId })
        assertEquals(setOf(health), index.history.single().labels)
        assertEquals(mapOf(money to 1f), index.projects.single().labelWeights)
    }

    @Test
    fun `a destination lends what its tasks carry beyond the project's own labels`() {
        val index = plan.build { floatArrayOf(1f, 0f) }

        // The one task in Soika carries only the project's own label.
        assertTrue(index.destinations[soika].orEmpty().isEmpty())
    }

    @Test
    fun `history knows where each task sits, and has a centroid per project`() {
        val index = plan.build { floatArrayOf(1f, 0f) }

        assertEquals(mapOf(100L to soika, 101L to null), index.history.associate { it.taskId to it.projectId })
        // The inbox task has no centroid to add to; Soika's one task is its centroid.
        assertEquals(setOf(soika), index.centroids.keys)
    }

    @Test
    fun `the runs of a repeating task count once for projects, every time for labels`() {
        val home = 20L
        val runs =
            workspace.copy(
                tasks =
                    workspace.tasks +
                        listOf(
                            SuggestionTaskFacts(110, "Вытереть пыль", soika, createdAt = 10),
                            SuggestionTaskFacts(111, "Вытереть пыль", soika, createdAt = 11),
                            SuggestionTaskFacts(112, "вытереть  пыль", soika, createdAt = 12),
                            SuggestionTaskFacts(113, "Вытереть пыль", home, createdAt = 13),
                        ),
                projects = workspace.projects + SuggestionProjectFacts(home, "Дом", ""),
            )

        val index = SuggestionIndexPlan(runs).build { floatArrayOf(1f, 0f) }

        // The newest run stands for the rest of its project; another project's run stays its own.
        assertEquals(
            setOf(100L, 112L, 113L),
            index.projectHistory.map { it.taskId }.toSet(),
        )
        assertEquals(setOf(100L, 101L, 110L, 111L, 112L, 113L), index.history.map { it.taskId }.toSet())
    }

    @Test
    fun `a project that is not open is left out with everything it holds`() {
        val closed =
            listOf(
                SuggestionProjectFacts(11, "Old", "", ProjectStatus.ARCHIVED),
                SuggestionProjectFacts(12, "Done", "", ProjectStatus.COMPLETED),
                SuggestionProjectFacts(13, "Dropped", "", ProjectStatus.CANCELLED),
            )
        val withClosed =
            workspace.copy(
                tasks = workspace.tasks + SuggestionTaskFacts(103, "Оплатить сервер", 11, createdAt = 4),
                taskLabels = workspace.taskLabels + LabelPair(103, money),
                projects = workspace.projects + closed,
                projectLabels = workspace.projectLabels + LabelPair(11, health),
            )
        val closedPlan = SuggestionIndexPlan(withClosed)

        val index = closedPlan.build { floatArrayOf(1f, 0f) }

        // Not a place, not a voter, not history, and not part of how a label is described.
        assertEquals(listOf(soika), index.projects.map { it.projectId })
        assertEquals(setOf(soika), closedPlan.projectDocuments.keys)
        assertEquals(setOf(100L, 101L), index.history.map { it.taskId }.toSet())
        assertEquals("title: здоровье | text: здоровье", closedPlan.labelTexts[health])
        assertTrue(closedPlan.taskTexts.none { it.first == 103L })
    }

    @Test
    fun `a key names the model as well as the text`() {
        assertEquals(SuggestionIndexPlan.key("a"), SuggestionIndexPlan.key("a"))
        assertNotEquals(SuggestionIndexPlan.key("a"), SuggestionIndexPlan.key("b"))
        assertEquals(64, SuggestionIndexPlan.key("a").length)
    }
}
