package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Scoring on hand-built two- and three-dimensional vectors, where which label
 * should win is obvious by inspection.
 */
class LabelScorerTest {
    private val day = 24L * 60 * 60 * 1000
    private val now = 1_000L * day

    private val money = 1L
    private val health = 2L
    private val work = 3L

    private fun v(vararg values: Float) = Vectors.prepare(values, dimensions = values.size)

    private val labels =
        listOf(
            LabelVector(money, v(1f, 0f, 0f)),
            LabelVector(health, v(0f, 1f, 0f)),
            LabelVector(work, v(0f, 0f, 1f)),
        )

    private fun task(
        id: Long,
        vector: FloatArray,
        vararg labels: Long,
        age: Long = 0,
    ) = HistoryTask(id, vector, labels.toSet(), now - age)

    @Test
    fun `nothing to name means nothing is scored`() {
        assertTrue(LabelScorer.score(v(1f, 0f, 0f), LabelSuggestionIndex.EMPTY, null, now).isEmpty())
    }

    @Test
    fun `a title next to labelled tasks takes their label`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects = emptyList(),
                history = listOf(task(10, v(0.2f, 1f, 0f), health), task(11, v(0.3f, 1f, 0f), health)),
            )

        val scores = LabelScorer.score(v(0f, 1f, 0f), index, projectId = null, now = now)

        assertEquals(listOf(health), LabelScorer.pick(scores, exclude = emptySet(), minScore = 0.4f))
        // Both neighbours voted health and nothing else: the full neighbour share,
        // plus nearly all of the label share, since the title sits on the label.
        assertEquals(LabelSuggestionRecipe.WEIGHT_NEIGHBOURS_WITHOUT_PROJECT + 0.3f, scores.getValue(health), 0.01f)
    }

    @Test
    fun `an unlabelled neighbour is a vote for nothing`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects = emptyList(),
                history = listOf(task(10, v(0f, 1f, 0f), health), task(11, v(0f, 1f, 0f))),
            )

        val scores = LabelScorer.score(v(0f, 1f, 0f), index, projectId = null, now = now)

        assertEquals(LabelSuggestionRecipe.WEIGHT_NEIGHBOURS_WITHOUT_PROJECT / 2 + 0.3f, scores.getValue(health), 0.01f)
    }

    @Test
    fun `an old neighbour counts for less than a recent one`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects = emptyList(),
                history =
                    listOf(
                        task(10, v(1f, 1f, 0f), money, age = LabelSuggestionRecipe.HALF_LIFE_MILLIS),
                        task(11, v(1f, 1f, 0f), health),
                    ),
            )

        val scores = LabelScorer.score(v(1f, 1f, 0f), index, projectId = null, now = now)

        // The recent vote weighs 1, the old one 1/2: health takes 2/3 of the neighbours.
        val neighbours = LabelSuggestionRecipe.WEIGHT_NEIGHBOURS_WITHOUT_PROJECT
        assertEquals(neighbours * 2 / 3 + 0.15f, scores.getValue(health), 0.01f)
        assertEquals(neighbours / 3 + 0.15f, scores.getValue(money), 0.01f)
    }

    @Test
    fun `the task being edited does not vote for itself`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects = emptyList(),
                history = listOf(task(10, v(0f, 1f, 0f), health)),
            )

        val scores = LabelScorer.score(v(0f, 1f, 0f), index, projectId = null, now = now, excludeTaskId = 10)

        assertEquals(0.3f, scores.getValue(health), 0.01f)
    }

    @Test
    fun `the label signal is relative, so an equally distant title favours none`() {
        val index = LabelSuggestionIndex(labels = labels, projects = emptyList(), history = emptyList())

        val scores = LabelScorer.score(v(1f, 1f, 1f), index, projectId = null, now = now)

        assertEquals(0.1f, scores.getValue(money), 0.001f)
        assertEquals(0.1f, scores.getValue(health), 0.001f)
        assertEquals(0.1f, scores.getValue(work), 0.001f)
    }

    @Test
    fun `a title that reads like a project gets the project's labels`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects =
                    listOf(
                        ProjectVector(100, v(1f, 0f, 1f), mapOf(work to 1f)),
                        ProjectVector(101, v(-1f, 1f, 0f), emptyMap()),
                    ),
                history = emptyList(),
            )

        val scores = LabelScorer.score(v(1f, 0f, 1f), index, projectId = null, now = now)

        // The title is the first project's own vector, so the project takes all of the share.
        assertEquals(0.15f + LabelSuggestionRecipe.WEIGHT_PROJECTS, scores.getValue(work), 0.01f)
    }

    @Test
    fun `a known project lends what its other tasks were labelled`() {
        val index =
            LabelSuggestionIndex(
                labels = labels,
                projects = emptyList(),
                history = emptyList(),
                destinations = mapOf(100L to mapOf(money to 0.5f)),
            )

        val inProject = LabelScorer.score(v(1f, 1f, 1f), index, projectId = 100, now = now)
        val elsewhere = LabelScorer.score(v(1f, 1f, 1f), index, projectId = null, now = now)

        assertEquals(0.1f + LabelSuggestionRecipe.WEIGHT_DESTINATION * 0.5f, inProject.getValue(money), 0.001f)
        assertEquals(0.1f, elsewhere.getValue(money), 0.001f)
    }

    @Test
    fun `picking skips what the task already has, keeps the best first, and stops at the cap`() {
        val scores = mapOf(1L to 0.9f, 2L to 0.8f, 3L to 0.7f, 4L to 0.6f, 5L to 0.1f)

        assertEquals(listOf(2L, 3L, 4L), LabelScorer.pick(scores, exclude = setOf(1L), minScore = 0.4f))
        assertEquals(listOf(1L, 2L), LabelScorer.pick(scores, exclude = emptySet(), minScore = 0.75f))
    }
}
