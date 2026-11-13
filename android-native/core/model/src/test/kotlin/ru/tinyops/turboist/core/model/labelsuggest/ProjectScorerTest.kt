package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Project scoring on hand-built three-dimensional vectors, where which project
 * a title belongs in is obvious by inspection.
 */
class ProjectScorerTest {
    private val day = 24L * 60 * 60 * 1000
    private val now = 1_000L * day

    private val home = 1L
    private val work = 2L
    private val archive = 3L

    private fun v(vararg values: Float) = Vectors.prepare(values, dimensions = values.size)

    private fun project(
        id: Long,
        vector: FloatArray,
        open: Boolean = true,
    ) = ProjectVector(id, vector, labelWeights = emptyMap(), open = open)

    private fun task(
        id: Long,
        vector: FloatArray,
        projectId: Long?,
        age: Long = 0,
    ) = HistoryTask(id, vector, emptySet(), now - age, projectId)

    private fun index(
        history: List<HistoryTask>,
        projects: List<ProjectVector> =
            listOf(project(home, v(1f, 0f, 0f)), project(work, v(0f, 1f, 0f))),
    ) = LabelSuggestionIndex(
        labels = emptyList(),
        projects = projects,
        history = history,
        centroids = projectCentroids(history),
    )

    @Test
    fun `no open project means nothing is scored`() {
        val index = index(emptyList(), projects = listOf(project(archive, v(1f, 0f, 0f), open = false)))

        assertTrue(ProjectScorer.score(v(1f, 0f, 0f), index, now).isEmpty())
    }

    @Test
    fun `a title next to a project's tasks is offered that project`() {
        val index =
            index(
                listOf(
                    task(10, v(0f, 0f, 1f), home),
                    task(11, v(0.1f, 0f, 1f), home),
                    task(12, v(0f, 1f, 0f), work),
                ),
            )

        val scores = ProjectScorer.score(v(0f, 0f, 1f), index, now)

        // The neighbours and the centroid both point home, whatever the project names say.
        assertEquals(listOf(home), ProjectScorer.pick(scores, exclude = emptySet()))
        assertTrue(scores.getValue(home) > 0.85f, "home scored ${scores.getValue(home)}")
    }

    @Test
    fun `inbox tasks do not vote`() {
        val index =
            index(
                listOf(
                    task(10, v(0f, 0f, 1f), null),
                    task(11, v(0f, 0f, 1f), null),
                    task(12, v(0.2f, 0f, 1f), work),
                ),
            )

        val scores = ProjectScorer.score(v(0f, 0f, 1f), index, now)

        assertEquals(listOf(work), ProjectScorer.pick(scores, exclude = emptySet()))
    }

    @Test
    fun `a project that is not open is never offered, but still takes its share`() {
        val history = listOf(task(10, v(0f, 0f, 1f), archive), task(11, v(0f, 1f, 0f), work))
        val index =
            index(
                history,
                projects =
                    listOf(
                        project(home, v(1f, 0f, 0f)),
                        project(work, v(0f, 1f, 0f)),
                        project(archive, v(0f, 0f, 1f), open = false),
                    ),
            )

        val scores = ProjectScorer.score(v(0f, 0f, 1f), index, now)

        assertEquals(setOf(home, work), scores.keys)
        // The archive soaked up the title's weight instead of lending it to an open project.
        assertEquals(emptyList(), ProjectScorer.pick(scores, exclude = emptySet()))
    }

    @Test
    fun `an old neighbour counts for less than a recent one`() {
        val index =
            index(
                listOf(
                    task(10, v(0f, 0f, 1f), home, age = 365 * day),
                    task(11, v(0f, 0f, 1f), work),
                ),
                projects = listOf(project(home, v(1f, 0f, 0f)), project(work, v(1f, 0f, 0f))),
            )

        val scores = ProjectScorer.score(v(0f, 0f, 1f), index, now)

        assertTrue(scores.getValue(work) > scores.getValue(home))
    }

    @Test
    fun `the place already chosen is left out and the answer is capped`() {
        val scores = mapOf(home to 0.9f, work to 0.5f, archive to 0.3f, 4L to 0.26f, 5L to 0.1f)

        assertEquals(listOf(work, archive, 4L), ProjectScorer.pick(scores, exclude = setOf(home)))
    }

    @Test
    fun `a centroid is the unit-length mean of a project's newest tasks`() {
        val history =
            listOf(
                task(10, v(1f, 0f, 0f), home),
                task(11, v(0f, 1f, 0f), home),
                task(12, v(0f, 0f, 1f), home, age = 10 * day),
                task(13, v(0f, 0f, 1f), null),
            )

        val centroids = projectCentroids(history, perProject = 2)

        assertEquals(setOf(home), centroids.keys)
        val c = centroids.getValue(home)
        assertEquals(0.7071f, c[0], 0.001f)
        assertEquals(0.7071f, c[1], 0.001f)
        assertEquals(0f, c[2], 0.001f)
    }
}
