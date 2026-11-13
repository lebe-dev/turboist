package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.test.Test
import kotlin.test.assertEquals

class SuggestionTextsTest {
    @Test
    fun `a title is asked as a classification query`() {
        assertEquals("task: classification | query: купить корм", SuggestionTexts.query("  купить\n корм "))
    }

    @Test
    fun `a label without a description or projects falls back to its name`() {
        assertEquals("title: книги | text: книги", SuggestionTexts.labelDocument("книги", "", emptyList()))
    }

    @Test
    fun `a label carries its description and the projects it is on`() {
        val document =
            SuggestionTexts.labelDocument(
                name = "здоровье",
                description = "Всё что связано\nсо здоровьем.",
                projects = listOf(ProjectText("Поддерживать тело в тонусе"), ProjectText("Забота", "Хорошо отдыхать")),
            )

        assertEquals(
            "title: здоровье | text: Всё что связано со здоровьем. " +
                "Projects: Поддерживать тело в тонусе; Забота (Хорошо отдыхать)",
            document,
        )
    }

    @Test
    fun `a project description that is only a link is left out`() {
        assertEquals(
            "title: Lab Board | text: Lab Board",
            SuggestionTexts.projectDocument(ProjectText("Lab Board", "https://jira.example.com/browse/KCP-1")),
        )
        assertEquals(
            "title: здоровье | text: Projects: Lab Board",
            SuggestionTexts.labelDocument("здоровье", "", listOf(ProjectText("Lab Board", "https://x.example"))),
        )
    }

    @Test
    fun `a project is its title and description`() {
        assertEquals(
            "title: Transmitter | text: WebUI для Transmission",
            SuggestionTexts.projectDocument(ProjectText("Transmitter", " WebUI для Transmission ")),
        )
    }
}
