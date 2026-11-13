package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.math.exp
import kotlin.math.pow

/**
 * How earlier tasks are asked: the [count] most similar ones vote, each vote
 * weighted by how similar the task is raised to [sharpness] — so the one nearly
 * identical task beats a crowd of loosely related ones — and halving every
 * [halfLifeMillis].
 */
internal data class NeighbourRule(
    val count: Int,
    val sharpness: Int,
    val halfLifeMillis: Long,
)

/** The arithmetic label and project suggestions share. */
internal object Scoring {
    /**
     * What the nearest earlier tasks vote for under [rule], normalised by the
     * total weight, so a neighbour that votes for nothing still dilutes the others.
     */
    fun neighbourVotes(
        query: FloatArray,
        history: List<HistoryTask>,
        now: Long,
        rule: NeighbourRule,
        votesOf: (HistoryTask) -> Iterable<Long>,
    ): Map<Long, Float> {
        if (history.isEmpty()) return emptyMap()
        val nearest =
            history
                .map { it to Vectors.dot(query, it.vector) }
                .sortedByDescending { it.second }
                .take(rule.count)
        val votes = HashMap<Long, Float>()
        var total = 0f
        for ((task, similarity) in nearest) {
            val age = (now - task.createdAt).coerceAtLeast(0)
            val weight =
                similarity.coerceAtLeast(0f).pow(rule.sharpness) *
                    0.5.pow(age.toDouble() / rule.halfLifeMillis).toFloat()
            total += weight
            for (id in votesOf(task)) votes[id] = votes.getOrDefault(id, 0f) + weight
        }
        if (total <= 0f) return emptyMap()
        return votes.mapValues { it.value / total }
    }

    /**
     * Turns similarities into shares that sum to one. Raw closeness is useless
     * as a score: every short title is roughly as close to everything, so only
     * how much closer it is to one than to the others says anything.
     */
    fun softmax(
        similarities: List<Float>,
        temperature: Float,
    ): FloatArray {
        if (similarities.isEmpty()) return FloatArray(0)
        val top = similarities.max()
        val exps = similarities.map { exp(((it - top) / temperature).toDouble()) }
        val sum = exps.sum()
        return FloatArray(exps.size) { (exps[it] / sum).toFloat() }
    }

    /** The best ids first: at most [max], none below [minScore], none of [exclude]; ties by id. */
    fun pick(
        scores: Map<Long, Float>,
        exclude: Set<Long>,
        minScore: Float,
        max: Int,
    ): List<Long> =
        scores.entries
            .filter { it.key !in exclude && it.value >= minScore }
            .sortedWith(compareByDescending<Map.Entry<Long, Float>> { it.value }.thenBy { it.key })
            .take(max)
            .map { it.key }
}
