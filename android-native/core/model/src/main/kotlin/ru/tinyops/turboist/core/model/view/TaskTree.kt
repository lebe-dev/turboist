package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus

/** A task together with the subtasks of it that were present in the same list. */
data class TaskNode(
    val task: Task,
    val children: List<TaskNode>,
)

/**
 * Nests a flat list of tasks by their parent links.
 *
 * Sibling order is the order of the input, so the ordering a list query already
 * decided survives nesting untouched.
 *
 * A task whose parent is not in the same list becomes a root. Lists are windows
 * — the today list holds a subtask whose parent is due next month — and a
 * subtask whose parent is out of frame must still be rendered rather than
 * silently dropped for pointing at something the window does not show.
 *
 * The same promise holds for rows that point at each other in a circle, which
 * the product cannot produce but a damaged row could: every task handed in comes
 * back out exactly once, and the walk terminates.
 */
fun buildTaskTree(tasks: List<Task>): List<TaskNode> {
    val childrenOf = HashMap<Long, MutableList<Task>>()
    val present = tasks.mapTo(HashSet()) { it.localId }
    val roots = mutableListOf<Task>()
    for (task in tasks) {
        val parent = task.parentLocalId
        if (parent != null && parent in present) {
            childrenOf.getOrPut(parent) { mutableListOf() }.add(task)
        } else {
            roots.add(task)
        }
    }
    val visited = HashSet<Long>()

    fun node(task: Task): TaskNode {
        visited.add(task.localId)
        val children =
            childrenOf[task.localId]
                .orEmpty()
                .filter { it.localId !in visited }
                .map(::node)
        return TaskNode(task, children)
    }

    val tree = roots.mapTo(mutableListOf(), ::node)
    // Tasks that formed a closed circle of parent links have no root to hang
    // from, so the first one met becomes one. Refusing to visit a task twice is
    // what stops the walk; adding what the walk never reached is what keeps the
    // damage to a misplaced row instead of a vanished one.
    for (task in tasks) {
        if (task.localId !in visited) tree += node(task)
    }
    return tree
}

/** Every task of these subtrees, parents before their own children. */
fun List<TaskNode>.flattenTasks(): List<Task> {
    val out = mutableListOf<Task>()

    fun walk(nodes: List<TaskNode>) {
        for (node in nodes) {
            out.add(node.task)
            walk(node.children)
        }
    }

    walk(this)
    return out
}

/** A list split into the work still in play and the work already finished. */
data class TaskSplit(
    val open: List<Task>,
    val done: List<Task>,
)

/**
 * Splits a flat list by the completion of each task's topmost ancestor within
 * that same list.
 *
 * The unit that moves to the "done" section of a screen is a whole subtree, not
 * a row: a completed parent takes its subtasks with it even when they are open,
 * and a completed subtask of an open parent stays under that parent where the
 * user can see what is left. Judging every row on its own status would tear such
 * a tree in half.
 */
fun splitByRootCompletion(tasks: List<Task>): TaskSplit {
    val byLocalId = tasks.associateBy { it.localId }
    val open = mutableListOf<Task>()
    val done = mutableListOf<Task>()
    for (task in tasks) {
        var root = task
        val seen = HashSet<Long>()
        while (seen.add(root.localId)) {
            val parent = root.parentLocalId?.let(byLocalId::get) ?: break
            root = parent
        }
        if (root.status == TaskStatus.COMPLETED) done.add(task) else open.add(task)
    }
    return TaskSplit(open, done)
}

/**
 * Splits a list for a screen that shows subtasks under their parents: the work
 * still to do first, by priority, and every finished task in the folded block.
 *
 * Unlike [splitByRootCompletion], a finished subtask is not left under its open
 * parent — it goes to [TaskSplit.done] with the rest of the finished work, where
 * the screen keeps it folded. A subtask of a finished task goes there too, since
 * the finished parent takes its subtree with it. Siblings are ordered by priority
 * (highest first, ties keep the order the list came in); top-level tasks keep the
 * order the query decided.
 */
fun splitForSubtaskDisplay(tasks: List<Task>): TaskSplit {
    val split = splitByRootCompletion(sortSubtasksByPriority(tasks))
    val (finished, stillOpen) = split.open.partition { it.status == TaskStatus.COMPLETED }
    // Keep the input order inside the folded block, finished subtasks included.
    val doneIds = (split.done + finished).map { it.localId }.toSet()
    return TaskSplit(open = stillOpen, done = sortSubtasksByPriority(tasks).filter { it.localId in doneIds })
}

/**
 * Reorders only the tasks that have a parent in the list, by priority; the tasks
 * without one stay exactly where they were.
 */
fun sortSubtasksByPriority(tasks: List<Task>): List<Task> {
    val present = tasks.mapTo(HashSet()) { it.localId }
    val isSubtask = { task: Task -> task.parentLocalId != null && task.parentLocalId in present }
    val sorted = tasks.filter(isSubtask).sortedBy { it.priority.rank() }.iterator()
    return tasks.map { if (isSubtask(it)) sorted.next() else it }
}

/** The order subtasks are read in: the most urgent first, an unknown level last. */
private val PRIORITY_ORDER = listOf(Priority.HIGH, Priority.MEDIUM, Priority.LOW, Priority.NONE, Priority.UNKNOWN)

private fun Priority.rank(): Int = PRIORITY_ORDER.indexOf(this)
