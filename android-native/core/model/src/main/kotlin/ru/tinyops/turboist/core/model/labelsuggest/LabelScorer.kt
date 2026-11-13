package ru.tinyops.turboist.core.model.labelsuggest

import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.HALF_LIFE_MILLIS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.NEIGHBOURS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.NEIGHBOUR_SHARPNESS
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe.SOFTMAX_TEMPERATURE
import kotlin.math.exp
import kotlin.math.pow

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
        val neighbours = neighbourVotes(query, index.history, now, excludeTaskId)
        val labelShares = softmax(index.labels.map { Vectors.dot(query, it.vector) })
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
    ): List<Long> =
        scores.entries
            .filter { it.key !in exclude && it.value >= minScore }
            .sortedWith(compareByDescending<Map.Entry<Long, Float>> { it.value }.thenBy { it.key })
            .take(max)
            .map { it.key }

    /**
     * The labels of the most similar earlier tasks, each vote weighted by how
     * similar the task is (sharpened) and how recent; normalised by the total
     * weight, so an unlabelled neighbour counts as a vote for nothing.
     */
    private fun neighbourVotes(
        query: FloatArray,
        history: List<HistoryTask>,
        now: Long,
        excludeTaskId: Long?,
    ): Map<Long, Float> {
        val candidates = history.filter { it.taskId != excludeTaskId }
        if (candidates.isEmpty()) return emptyMap()
        val nearest =
            candidates
                .map { it to Vectors.dot(query, it.vector) }
                .sortedByDescending { it.second }
                .take(NEIGHBOURS)
        val votes = HashMap<Long, Float>()
        var total = 0f
        for ((task, similarity) in nearest) {
            val age = (now - task.createdAt).coerceAtLeast(0)
            val weight =
                similarity.coerceAtLeast(0f).pow(NEIGHBOUR_SHARPNESS) *
                    0.5.pow(age.toDouble() / HALF_LIFE_MILLIS).toFloat()
            total += weight
            for (label in task.labels) votes[label] = votes.getOrDefault(label, 0f) + weight
        }
        if (total <= 0f) return emptyMap()
        return votes.mapValues { it.value / total }
    }

    private fun projectVotes(
        query: FloatArray,
        projects: List<ProjectVector>,
    ): Map<Long, Float> {
        if (projects.isEmpty()) return emptyMap()
        val shares = softmax(projects.map { Vectors.dot(query, it.vector) })
        val votes = HashMap<Long, Float>()
        projects.forEachIndexed { i, project ->
            for ((label, weight) in project.labelWeights) {
                votes[label] = votes.getOrDefault(label, 0f) + shares[i] * weight
            }
        }
        return votes
    }

    private fun softmax(similarities: List<Float>): FloatArray {
        if (similarities.isEmpty()) return FloatArray(0)
        val top = similarities.max()
        val exps = similarities.map { exp(((it - top) / SOFTMAX_TEMPERATURE).toDouble()) }
        val sum = exps.sum()
        return FloatArray(exps.size) { (exps[it] / sum).toFloat() }
    }
}
