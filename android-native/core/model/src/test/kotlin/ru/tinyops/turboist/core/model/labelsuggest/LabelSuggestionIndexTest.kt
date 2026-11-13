package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.test.Test
import kotlin.test.assertEquals

class LabelSuggestionIndexTest {
    private val project = 100L
    private val bare = 101L
    private val own = 1L
    private val frequent = 2L
    private val rare = 3L

    private fun tasks(vararg labels: Set<Long>) =
        labels.mapIndexed { i, set -> TaskLabelFacts(taskId = i.toLong(), projectId = project, labels = set) }

    @Test
    fun `a project stands for its own labels and the ones most of its tasks carry`() {
        val members =
            tasks(
                setOf(own, frequent),
                setOf(frequent),
                setOf(rare),
                setOf(),
                setOf(),
                setOf(),
                setOf(),
                setOf(),
                setOf(),
                setOf(),
            )

        val weights = projectLabelWeights(listOf(project, bare), mapOf(project to setOf(own)), members)

        // frequent is on 2 of 10 tasks — under the 30 % bar — so only the project's own label counts.
        assertEquals(mapOf(own to 1f), weights[project])
        assertEquals(emptyMap(), weights[bare])
    }

    @Test
    fun `a label on a third of a project's tasks is one it stands for`() {
        val members = tasks(setOf(frequent), setOf(frequent), setOf(), setOf(), setOf(), setOf())

        val weights = projectLabelWeights(listOf(project), emptyMap(), members)

        assertEquals(mapOf(frequent to 2f / 6), weights[project])
    }

    @Test
    fun `a destination lends only the labels its project does not already give`() {
        val members = tasks(setOf(own, frequent), setOf(own, frequent), setOf(own), setOf(own, rare), setOf(own))

        val priors = destinationPriors(mapOf(project to setOf(own)), members)

        // Five tasks plus five of doubt.
        assertEquals(mapOf(frequent to 0.2f, rare to 0.1f), priors[project])
    }
}
