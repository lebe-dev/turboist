package ru.tinyops.turboist.nativeapp.quickadd

import org.junit.Test
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import kotlin.test.assertEquals

/**
 * The two title rules, as the device reads them.
 *
 * The cases below are the web client's own, restated: the two implementations
 * answer the same question about the same rules document, and a user who types a
 * title in a browser and the same title on a phone has to be offered the same
 * projects and warned about the same labels. Where they can drift they will, so
 * the cases travel together.
 *
 * `ignoreCase` is read straight, because the rules document always carries the
 * field: the server writes it on every rule, so there is no absent case to
 * decide. A rule that asks for case-insensitivity gets it; every other rule is
 * matched exactly.
 */
class MaskRulesTest {
    private fun rule(
        mask: String,
        projectIds: List<Long>,
        ignoreCase: Boolean = true,
    ) = ProjectSuggestionRule(mask = mask, projectIds = projectIds, ignoreCase = ignoreCase)

    private fun labelRule(
        mask: String,
        labelIds: List<Long>,
        ignoreCase: Boolean = true,
    ) = AutoLabelRule(mask = mask, labelIds = labelIds, ignoreCase = ignoreCase)

    private val projects =
        listOf(
            project(1, "Zebra"),
            project(2, "Alpha"),
            project(3, "Mango"),
            project(4, "Beta"),
            project(5, "Done", status = ProjectStatus.COMPLETED),
        )

    private fun titles(matched: List<Project>) = matched.map { it.title }

    // --- projects the title brings to mind ---

    @Test
    fun `nothing is offered when no rule matches`() {
        assertEquals(emptyList(), matchProjectSuggestions(listOf(rule("deploy", listOf(1))), "buy milk", projects))
    }

    @Test
    fun `a rule that asks to ignore case matches whatever the title was typed in`() {
        val offered = matchProjectSuggestions(listOf(rule("deploy", listOf(1))), "DEPLOY api", projects)

        assertEquals(listOf("Zebra"), titles(offered))
    }

    @Test
    fun `a case-sensitive rule matches only the spelling it names`() {
        val rules = listOf(rule("Deploy", listOf(1), ignoreCase = false))

        assertEquals(emptyList(), matchProjectSuggestions(rules, "deploy api", projects))
        assertEquals(listOf(1L), matchProjectSuggestions(rules, "Deploy api", projects).map { it.localId })
    }

    @Test
    fun `suggestions are sorted by title`() {
        val offered = matchProjectSuggestions(listOf(rule("x", listOf(1, 2, 3))), "x", projects)

        assertEquals(listOf("Alpha", "Mango", "Zebra"), titles(offered))
    }

    @Test
    fun `no more than a row of suggestions is ever offered`() {
        val offered = matchProjectSuggestions(listOf(rule("x", listOf(1, 2, 3, 4))), "x", projects)

        assertEquals(MAX_PROJECT_SUGGESTIONS, offered.size)
        assertEquals(listOf("Alpha", "Beta", "Mango"), titles(offered))
    }

    @Test
    fun `several matching rules are one deduplicated set`() {
        val rules = listOf(rule("buy", listOf(2, 3)), rule("milk", listOf(3, 4)))

        assertEquals(listOf("Alpha", "Beta", "Mango"), titles(matchProjectSuggestions(rules, "buy milk", projects)))
    }

    @Test
    fun `a finished project and one this device has never seen are both skipped`() {
        val offered = matchProjectSuggestions(listOf(rule("x", listOf(5, 999, 2))), "x", projects)

        assertEquals(listOf(2L), offered.map { it.localId })
    }

    @Test
    fun `the project the draft already points at is not offered back`() {
        val offered =
            matchProjectSuggestions(listOf(rule("x", listOf(1, 2, 3, 4))), "x", projects, excludeLocalIds = setOf(2))

        assertEquals(listOf("Beta", "Mango", "Zebra"), titles(offered))
    }

    @Test
    fun `a rule with an empty mask matches nothing rather than everything`() {
        assertEquals(emptyList(), matchProjectSuggestions(listOf(rule("", listOf(1))), "anything", projects))
    }

    /**
     * A project created on this device and not yet sent has no server id, and the
     * rules name projects by server id — so it cannot be named by a rule, and is
     * left out rather than matched by accident against another project's id.
     */
    @Test
    fun `a project the server has never seen cannot be suggested`() {
        val unsent = listOf(project(7, "Unsent", serverId = null))

        assertEquals(emptyList(), matchProjectSuggestions(listOf(rule("x", listOf(7))), "x", unsent))
    }

    // --- labels the title earns ---

    private val labels = listOf(label(1, "urgent"), label(2, "errand"), label(3, "home"))

    @Test
    fun `a matching rule names the labels that will be attached`() {
        val rules = listOf(labelRule("buy", listOf(2, 3)))

        assertEquals(listOf("errand", "home"), matchAutoLabelNames(rules, "buy milk", labels))
    }

    @Test
    fun `a label already chosen by hand or already rejected is not offered again`() {
        val rules = listOf(labelRule("buy", listOf(1, 2, 3)))

        val offered = matchAutoLabelNames(rules, "buy milk", labels, exclude = setOf("urgent", "home"))

        assertEquals(listOf("errand"), offered)
    }

    @Test
    fun `a rule naming a label this device has never seen contributes nothing`() {
        val rules = listOf(labelRule("buy", listOf(404)))

        assertEquals(emptyList(), matchAutoLabelNames(rules, "buy milk", labels))
    }

    @Test
    fun `a case-sensitive label rule matches only the spelling it names`() {
        val rules = listOf(labelRule("Urgent", listOf(1), ignoreCase = false))

        assertEquals(emptyList(), matchAutoLabelNames(rules, "urgent thing", labels))
        assertEquals(listOf("urgent"), matchAutoLabelNames(rules, "Urgent thing", labels))
    }
}
