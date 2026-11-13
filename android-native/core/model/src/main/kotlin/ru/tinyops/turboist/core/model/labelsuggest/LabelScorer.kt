package ru.tinyops.turboist.core.model.labelsuggest

import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.HALF_LIFE_MILLIS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.NEIGHBOURS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.NEIGHBOUR_SHARPNESS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.SOFTMAX_TEMPERATURE

/**
 * Scores every label for one title, from an index built beforehand.
 *
 * Pure arithmetic over vectors: the title has already been through the model,
 * so this runs in well under a millisecond for a workspace of thousands of
 * tasks and can be asked on every pause in typing.
 */
object LabelScorer {
    /**
     * Each label's score for [query], from 0 to 1.
     *
     * @param query the title's embedding, already prepared with [Vectors.prepare].
     * @param projectId the project the task is going into or is in, if any.
     * @param now the moment the neighbours' age is measured from.
     * @param excludeTaskId a task that must not vote for itself — the one being edited.
     */
    fun score(
        query: FloatArray,
        index: LabelSuggestionIndex,
        projectId: Long?,
        now: Long,
        excludeTaskId: Long? = null,
    ): Map<Long, Float> {
        if (index.isEmpty) return emptyMap()
        val neighbours =
            Scoring.neighbourVotes(
                query = query,
                history = index.history.filter { it.taskId != excludeTaskId },
                now = now,
                rule = NeighbourRule(NEIGHBOURS, NEIGHBOUR_SHARPNESS, HALF_LIFE_MILLIS),
                votesOf = HistoryTask::labels,
            )
        val labelShares = Scoring.softmax(index.labels.map { Vectors.dot(query, it.vector) }, SOFTMAX_TEMPERATURE)
        val projectVotes = projectVotes(query, index.projects)
        val destination = projectId?.let { index.destinations[it] }.orEmpty()
        val neighbourWeight =
            if (projectId != null) {
                LabelSuggestionRecipe.WEIGHT_NEIGHBOURS
            } else {
                LabelSuggestionRecipe.WEIGHT_NEIGHBOURS_WITHOUT_PROJECT
            }
        return index.labels.withIndex().associate { (i, label) ->
            val id = label.labelId
            var score =
                neighbourWeight * neighbours.getOrDefault(id, 0f) +
                    LabelSuggestionRecipe.WEIGHT_LABELS * labelShares[i] +
                    LabelSuggestionRecipe.WEIGHT_PROJECTS * projectVotes.getOrDefault(id, 0f)
            if (projectId != null) score += LabelSuggestionRecipe.WEIGHT_DESTINATION * destination.getOrDefault(id, 0f)
            id to score
        }
    }

    /**
     * The labels worth offering, best first: at most [max], none scoring below
     * [minScore], and none of [exclude] — the labels already on the task, the
     * ones its project or a title rule gives it, the ones the user took off.
     */
    fun pick(
        scores: Map<Long, Float>,
        exclude: Set<Long>,
        minScore: Float,
        max: Int = LabelSuggestionRecipe.MAX_SUGGESTIONS,
    ): List<Long> = Scoring.pick(scores, exclude, minScore, max)

    private fun projectVotes(
        query: FloatArray,
        projects: List<ProjectVector>,
    ): Map<Long, Float> {
        if (projects.isEmpty()) return emptyMap()
        val shares = Scoring.softmax(projects.map { Vectors.dot(query, it.vector) }, SOFTMAX_TEMPERATURE)
        val votes = HashMap<Long, Float>()
        projects.forEachIndexed { i, project ->
            for ((label, weight) in project.labelWeights) {
                votes[label] = votes.getOrDefault(label, 0f) + shares[i] * weight
            }
        }
        return votes
    }
}
