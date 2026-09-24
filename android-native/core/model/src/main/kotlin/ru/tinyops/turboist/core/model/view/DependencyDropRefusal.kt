package ru.tinyops.turboist.core.model.view

/**
 * Why a subtask dropped onto another cannot be made to wait for it.
 *
 * The first three follow from the subtask tree: a blocker is inherited down the
 * tree, so a wait along one branch would block a task on its own work or be
 * meaningless, and a finished task holds nothing up. The last two are the
 * server's own refusals of a new `blocks` edge.
 */
enum class DependencyDropRefusal {
    ANCESTOR,
    DESCENDANT,
    COMPLETED,
    EXISTS,
    CYCLE,
}

/**
 * Whether dropping [draggedLocalId] onto [targetLocalId] — "the dragged task is
 * blocked by the target" — would be refused, and why; `null` when it may go ahead.
 *
 * [parentOf] maps each task in the subtree to the task above it. [blockEdges]
 * must be every `blocks` edge the device holds, whatever the state of its ends:
 * the server refuses a duplicate and a loop regardless of either.
 */
fun dependencyDropRefusal(
    draggedLocalId: Long,
    targetLocalId: Long,
    parentOf: Map<Long, Long>,
    targetOpen: Boolean,
    blockEdges: Collection<BlockEdge>,
): DependencyDropRefusal? {
    if (!targetOpen) return DependencyDropRefusal.COMPLETED
    if (targetLocalId in ancestorsOf(draggedLocalId, parentOf)) return DependencyDropRefusal.ANCESTOR
    if (draggedLocalId in ancestorsOf(targetLocalId, parentOf)) return DependencyDropRefusal.DESCENDANT
    if (blockEdges.any { it.blockerLocalId == targetLocalId && it.blockedLocalId == draggedLocalId }) {
        return DependencyDropRefusal.EXISTS
    }
    if (wouldCloseBlockingCycle(targetLocalId, draggedLocalId, blockEdges)) return DependencyDropRefusal.CYCLE
    return null
}

private fun ancestorsOf(
    localId: Long,
    parentOf: Map<Long, Long>,
): Set<Long> {
    val out = LinkedHashSet<Long>()
    var parent = parentOf[localId]
    while (parent != null && out.add(parent)) {
        parent = parentOf[parent]
    }
    return out
}
