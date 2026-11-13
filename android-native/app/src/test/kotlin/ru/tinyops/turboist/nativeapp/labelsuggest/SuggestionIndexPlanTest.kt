package ru.tinyops.turboist.nativeapp.labelsuggest

import org.junit.Test
import ru.tinyops.turboist.core.database.dao.LabelPair
import ru.tinyops.turboist.core.database.dao.SuggestionLabelFacts
import ru.tinyops.turboist.core.database.dao.SuggestionProjectFacts
import ru.tinyops.turboist.core.database.dao.SuggestionTaskFacts
import ru.tinyops.turboist.core.database.dao.SuggestionWorkspace
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
    fun `a key names the model as well as the text`() {
        assertEquals(SuggestionIndexPlan.key("a"), SuggestionIndexPlan.key("a"))
        assertNotEquals(SuggestionIndexPlan.key("a"), SuggestionIndexPlan.key("b"))
        assertEquals(64, SuggestionIndexPlan.key("a").length)
    }
}
