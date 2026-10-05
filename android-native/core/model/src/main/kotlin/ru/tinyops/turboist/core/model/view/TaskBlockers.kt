package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.TaskRelationSummary

/** One directed `blocks` edge: [blockerLocalId] stands in the way of [blockedLocalId]. */
data class BlockEdge(
    val blockerLocalId: Long,
    val blockedLocalId: Long,
)

/**
 * How many still-open tasks stand in the way of each of [taskLocalIds].
 *
 * A blocker is inherited down the subtask tree: a task whose parent is blocked
 * is blocked as well, because finishing part of work that cannot start is not
 * progress. The one exception is an inherited blocker that lives inside the
 * task's own subtree — that is the work itself, and counting it would make a
 * parent impossible to finish by its own children. A blocker named on the task
 * directly always counts, wherever it sits.
 *
 * The rule is the server's, and completion is refused by it whatever the screen
 * shows. It is repeated here so a list can draw the refusal on the checkbox
 * before the user taps it, instead of letting the tap look accepted and be
 * taken back later.
 *
 * @param openBlockEdges every `blocks` edge whose blocker is still open. Edges
 *   whose blocker is completed or cancelled must be left out by the caller:
 *   either state releases what the task was holding up.
 * @param parentOf each subtask's parent. Tasks with no parent are simply absent.
 * @return a count per task, with the tasks nothing blocks left out entirely.
 */
fun blockedByOpenCounts(
    taskLocalIds: Collection<Long>,
    openBlockEdges: Collection<BlockEdge>,
    parentOf: Map<Long, Long>,
): Map<Long, Int> {
    if (openBlockEdges.isEmpty()) return emptyMap()
    val directBlockers = directBlockersOf(openBlockEdges)
    val counts = HashMap<Long, Int>()
    for (taskLocalId in taskLocalIds.toSet()) {
        val blockers = blockersOf(taskLocalId, directBlockers, parentOf)
        if (blockers.isNotEmpty()) counts[taskLocalId] = blockers.size
    }
    return counts
}

/**
 * Which still-open tasks stand in the way of one task, nearest claim first.
 *
 * The same rule the counts are built from, answered by name instead of by
 * number, because a screen that refuses a completion has to say what is holding
 * it up — a count alone leaves the user with nowhere to go.
 *
 * The order is the one a reader expects to be told about: what this task itself
 * names, and then what it has inherited from the work above it.
 */
fun openBlockerLocalIds(
    taskLocalId: Long,
    openBlockEdges: Collection<BlockEdge>,
    parentOf: Map<Long, Long>,
): List<Long> {
    if (openBlockEdges.isEmpty()) return emptyList()
    return blockersOf(taskLocalId, directBlockersOf(openBlockEdges), parentOf).toList()
}

/** The blockers each task names on its own, before anything is inherited. */
private fun directBlockersOf(openBlockEdges: Collection<BlockEdge>): Map<Long, Set<Long>> {
    val direct = HashMap<Long, MutableSet<Long>>()
    for (edge in openBlockEdges) {
        direct.getOrPut(edge.blockedLocalId) { LinkedHashSet() }.add(edge.blockerLocalId)
    }
    return direct
}

/** One task's blockers: its own, plus its ancestors' minus anything inside its own subtree. */
private fun blockersOf(
    taskLocalId: Long,
    directBlockers: Map<Long, Set<Long>>,
    parentOf: Map<Long, Long>,
): Set<Long> {
    val blockers = LinkedHashSet<Long>()
    blockers += directBlockers[taskLocalId].orEmpty()
    for (ancestor in ancestorsOf(taskLocalId, parentOf)) {
        for (blocker in directBlockers[ancestor].orEmpty()) {
            if (coversSubtree(taskLocalId, blocker, parentOf)) continue
            blockers += blocker
        }
    }
    return blockers
}

/**
 * The rollup every list row carries, for the tasks a list is about to draw.
 *
 * [relationCounts] is the task's own relations, both directions and both kinds.
 * Inherited blockers are deliberately not added to it: they belong to an
 * ancestor, and a row claiming links it does not have would send the reader
 * looking for them.
 */
fun taskRelationSummaries(
    taskLocalIds: Collection<Long>,
    openBlockEdges: Collection<BlockEdge>,
    relationCounts: Map<Long, Int>,
    parentOf: Map<Long, Long>,
): Map<Long, TaskRelationSummary> {
    val blocked = blockedByOpenCounts(taskLocalIds, openBlockEdges, parentOf)
    val summaries = HashMap<Long, TaskRelationSummary>()
    for (taskLocalId in taskLocalIds.toSet()) {
        val blockedBy = blocked[taskLocalId] ?: 0
        val total = relationCounts[taskLocalId] ?: 0
        if (blockedBy == 0 && total == 0) continue
        summaries[taskLocalId] = TaskRelationSummary(blockedByOpen = blockedBy, total = total)
    }
    return summaries
}

/**
 * The chain of parents above a task, nearest first.
 *
 * The walk remembers where it has been. A parent chain cannot loop in the
 * product, but this reads rows copied from a server, and a replica somehow left
 * with a loop in it must still draw a list rather than spin.
 */
private fun ancestorsOf(
    taskLocalId: Long,
    parentOf: Map<Long, Long>,
): List<Long> {
    val out = mutableListOf<Long>()
    val seen = mutableSetOf(taskLocalId)
    var parent = parentOf[taskLocalId]
    while (parent != null && seen.add(parent)) {
        out += parent
        parent = parentOf[parent]
    }
    return out
}

/** True when [rootLocalId] is [taskLocalId] itself or sits somewhere above it. */
private fun coversSubtree(
    rootLocalId: Long,
    taskLocalId: Long,
    parentOf: Map<Long, Long>,
): Boolean = rootLocalId == taskLocalId || rootLocalId in ancestorsOf(taskLocalId, parentOf)
