package ru.tinyops.turboist.core.model.view

/**
 * Why a subtask dropped onto another cannot be nested under it (made its
 * subtask). Nesting under one of the dragged task's own ancestors is fine —
 * that is just moving it up a level — so only the descendant case is a real
 * cycle, plus the same "nothing left to hold" refusal a finished target gives.
 */
enum class NestDropRefusal {
    DESCENDANT,
    COMPLETED,
}

/**
 * Whether nesting [draggedLocalId] under [targetLocalId] — making it
 * [targetLocalId]'s subtask — would change nothing: [targetLocalId] is already
 * its parent. Not a refusal, just nothing to offer, the same way the dragged
 * row itself is not offered as its own target.
 */
fun isNestNoop(
    draggedLocalId: Long,
    targetLocalId: Long,
    parentOf: Map<Long, Long>,
): Boolean = parentOf[draggedLocalId] == targetLocalId

/**
 * Whether dropping [draggedLocalId] onto [targetLocalId] to nest it there would
 * be refused, and why; `null` when it may go ahead (see [isNestNoop] for the
 * no-op case, checked separately since it is not really a refusal).
 *
 * [parentOf] maps each task in the subtree to the task above it.
 */
fun nestDropRefusal(
    draggedLocalId: Long,
    targetLocalId: Long,
    parentOf: Map<Long, Long>,
    targetOpen: Boolean,
): NestDropRefusal? {
    if (!targetOpen) return NestDropRefusal.COMPLETED
    // targetLocalId sitting inside dragged's own subtree would nest it under its
    // own descendant — the one cycle the tree can never resolve.
    if (draggedLocalId in ancestorsOf(targetLocalId, parentOf)) return NestDropRefusal.DESCENDANT
    return null
}

// A private copy of the same parent-chain walk DependencyDropRefusal.kt does:
// Kotlin's file-private visibility means the two cannot share one function
// without a visibility change that would collide with the differently-shaped
// `ancestorsOf` TaskBlockers.kt already declares in this package.
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
