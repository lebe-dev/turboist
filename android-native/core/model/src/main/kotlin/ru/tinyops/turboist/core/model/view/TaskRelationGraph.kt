package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.RelationDirection
import ru.tinyops.turboist.core.model.RelationType

/**
 * The three things a person says about two pieces of work, over the two things
 * the store records.
 *
 * One stored kind of edge carries two of them: a `blocks` edge read from the end
 * that is waiting is "blocked by", and read from the end that is holding it up is
 * "blocks". The third kind has no direction at all — saying two tasks are related
 * says the same thing from either side — so its direction is recorded once and
 * never read.
 */
enum class TaskRelationGroup(
    val type: RelationType,
    val direction: RelationDirection,
) {
    /** The peer has to happen before this task can be finished. */
    BLOCKED_BY(RelationType.BLOCKS, RelationDirection.INCOMING),

    /** This task has to happen before the peer can be finished. */
    BLOCKS(RelationType.BLOCKS, RelationDirection.OUTGOING),

    /** The two are worth reading together, and neither waits for the other. */
    RELATED(RelationType.RELATED, RelationDirection.OUTGOING),
    ;

    companion object {
        /**
         * Which of the three an edge belongs to, read from one of its ends.
         *
         * `null` for an edge whose kind this build does not know: a newer server
         * may name a kind that did not exist when this app was written, and the
         * honest answer is to leave it out of the three groups rather than to
         * file it under one of them.
         */
        fun of(
            type: RelationType,
            direction: RelationDirection,
        ): TaskRelationGroup? =
            when {
                type == RelationType.RELATED -> RELATED
                type != RelationType.BLOCKS -> null
                direction == RelationDirection.INCOMING -> BLOCKED_BY
                direction == RelationDirection.OUTGOING -> BLOCKS
                else -> null
            }
    }
}

/** The two ends of a stored relation, in the order the row records them. */
data class RelationEnds(
    val sourceLocalId: Long,
    val targetLocalId: Long,
)

/**
 * Which end of a new link is its source.
 *
 * A `blocks` edge is directed, and the direction is read relative to the task
 * the user acted on: incoming means the peer is what has to happen first.
 *
 * A `related` edge is symmetric and therefore has no natural source, so the pair
 * is put in a fixed order. That ordering is the whole reason the same two tasks
 * linked from either side are one edge rather than two: the store can only see a
 * duplicate if both attempts write the same row.
 */
fun relationEnds(
    taskLocalId: Long,
    otherTaskLocalId: Long,
    type: RelationType,
    direction: RelationDirection,
): RelationEnds =
    when {
        type != RelationType.BLOCKS ->
            RelationEnds(minOf(taskLocalId, otherTaskLocalId), maxOf(taskLocalId, otherTaskLocalId))

        direction == RelationDirection.INCOMING -> RelationEnds(otherTaskLocalId, taskLocalId)
        else -> RelationEnds(taskLocalId, otherTaskLocalId)
    }

/**
 * Whether saying "[blockerLocalId] has to happen before [blockedLocalId]" would
 * close a loop.
 *
 * A loop in the waiting graph is a deadlock with no way out from inside the
 * product: every task in it waits for the next one, so none of them can ever be
 * ticked off. It is refused when the link is made, which is the only moment
 * there is still something to refuse.
 *
 * The question is asked forwards: is the proposed blocker already waiting on the
 * task it would now hold up, at any remove. Only `blocks` edges take part —
 * an informational link makes nobody wait, so it can never deadlock anything and
 * must stay addable between any two tasks.
 *
 * @param blockEdges every `blocks` edge in the workspace, whatever state its
 *   ends are in. A finished blocker still counts here: it releases what it was
 *   holding up, but the edge remains, and reopening the task would bring the
 *   loop back.
 */
fun wouldCloseBlockingCycle(
    blockerLocalId: Long,
    blockedLocalId: Long,
    blockEdges: Collection<BlockEdge>,
): Boolean {
    if (blockerLocalId == blockedLocalId) return true
    val waitsFor = blockEdges.groupBy({ it.blockerLocalId }, { it.blockedLocalId })
    val seen = mutableSetOf(blockedLocalId)
    val queue = ArrayDeque<Long>()
    queue += blockedLocalId
    while (queue.isNotEmpty()) {
        for (next in waitsFor[queue.removeFirst()].orEmpty()) {
            if (next == blockerLocalId) return true
            if (seen.add(next)) queue += next
        }
    }
    return false
}
