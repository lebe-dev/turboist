package ru.tinyops.turboist.core.model.labelsuggest

import ru.tinyops.turboist.core.model.labelsuggest.ProjectSuggestionRecipe.HALF_LIFE_MILLIS
import ru.tinyops.turboist.core.model.labelsuggest.ProjectSuggestionRecipe.NEIGHBOURS
import ru.tinyops.turboist.core.model.labelsuggest.ProjectSuggestionRecipe.NEIGHBOUR_SHARPNESS
import ru.tinyops.turboist.core.model.labelsuggest.ProjectSuggestionRecipe.SOFTMAX_TEMPERATURE

/**
 * Scores every project in the index — the open ones — as the place for one
 * title, from an index built beforehand. Pure arithmetic over vectors, like
 * [LabelScorer].
 */
object ProjectScorer {
    /**
     * Each project's score for [query], from 0 to 1.
     *
     * @param query the title's embedding, already prepared with [Vectors.prepare].
     * @param now the moment the neighbours' age is measured from.
     */
    fun score(
        query: FloatArray,
        index: LabelSuggestionIndex,
        now: Long,
    ): Map<Long, Float> {
        if (index.projects.isEmpty()) return emptyMap()
        val neighbours =
            Scoring.neighbourVotes(
                query = query,
                // Inbox tasks are where nothing was decided yet: they say nothing about a place.
                history = index.history.filter { it.projectId != null },
                now = now,
                rule = NeighbourRule(NEIGHBOURS, NEIGHBOUR_SHARPNESS, HALF_LIFE_MILLIS),
                votesOf = { listOfNotNull(it.projectId) },
            )
        val documents = shares(query, index.projects.associate { it.projectId to it.vector })
        val centroids = shares(query, index.centroids)
        return index.projects.associate { project ->
            val id = project.projectId
            id to
                ProjectSuggestionRecipe.WEIGHT_NEIGHBOURS * neighbours.getOrDefault(id, 0f) +
                ProjectSuggestionRecipe.WEIGHT_CENTROIDS * centroids.getOrDefault(id, 0f) +
                ProjectSuggestionRecipe.WEIGHT_DOCUMENTS * documents.getOrDefault(id, 0f)
        }
    }

    /**
     * The projects worth offering, best first: at most [max], none scoring
     * below [minScore], and none of [exclude] — the place the task is already
     * headed for, and the ones a title rule already offers.
     */
    fun pick(
        scores: Map<Long, Float>,
        exclude: Set<Long>,
        minScore: Float = ProjectSuggestionRecipe.CAPTURE_THRESHOLD,
        max: Int = ProjectSuggestionRecipe.MAX_SUGGESTIONS,
    ): List<Long> = Scoring.pick(scores, exclude, minScore, max)

    private fun shares(
        query: FloatArray,
        vectors: Map<Long, FloatArray>,
    ): Map<Long, Float> {
        if (vectors.isEmpty()) return emptyMap()
        val ids = vectors.keys.toList()
        val shares = Scoring.softmax(ids.map { Vectors.dot(query, vectors.getValue(it)) }, SOFTMAX_TEMPERATURE)
        return ids.withIndex().associate { (i, id) -> id to shares[i] }
    }
}
