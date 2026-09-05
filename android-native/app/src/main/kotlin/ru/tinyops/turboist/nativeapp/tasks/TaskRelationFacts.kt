package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.database.dao.TaskRelationEdge
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.view.BlockEdge

/**
 * The `blocks` edges that still hold something up: the task at the blocking end
 * is open.
 *
 * This and [relationCountsOf] turn the stored edges into the two facts the
 * shared relation rule reads. They are written once and used by every screen
 * that draws a padlock or a link count, because two readings of the same rows
 * would eventually stop agreeing — and the moment they did, a list would offer a
 * tick the completion guard refuses, or refuse one it would have allowed.
 *
 * A blocker that was completed — or cancelled, which settles it just as finally
 * — releases what it was blocking, so its edge is left out here rather than
 * being weighed up again further along.
 */
fun openBlockEdgesOf(edges: List<TaskRelationEdge>): List<BlockEdge> =
    edges
        .filter { it.type == RelationType.BLOCKS && it.sourceOpen }
        .map { BlockEdge(blockerLocalId = it.sourceTaskLocalId, blockedLocalId = it.targetTaskLocalId) }

/**
 * How many links each task has of its own, both directions and both kinds.
 *
 * Only its own: a blocker inherited from the work above it is a real constraint
 * but not a link on this task, and counting it would send a reader looking for
 * something the task does not list.
 */
fun relationCountsOf(edges: List<TaskRelationEdge>): Map<Long, Int> {
    val counts = HashMap<Long, Int>()
    for (edge in edges) {
        counts[edge.sourceTaskLocalId] = (counts[edge.sourceTaskLocalId] ?: 0) + 1
        counts[edge.targetTaskLocalId] = (counts[edge.targetTaskLocalId] ?: 0) + 1
    }
    return counts
}
