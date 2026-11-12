package ru.tinyops.turboist.nativeapp.tasks

import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.sync.write.TaskEdit

/**
 * The open subtasks, at any depth, that carrying [root] to [part] has to take along.
 *
 * The phase blocks are cut task by task, so a subtask can be listed in a block of
 * its own; the tree is therefore walked across every phase block rather than
 * inside one. Only phase blocks are read: work in any other block (overdue, say)
 * is not planned for the day, and a new phase would not change that. A subtask
 * already in [part], and a finished one, stays as it is.
 */
fun subtasksToRepart(
    sections: List<TaskListSection>,
    root: Task,
    part: DayPart,
): List<Task> {
    val byParent =
        sections
            .filter { it.heading is SectionHeading.Phase }
            .flatMap { section -> section.rows.map { it.task } }
            .filter { it.parentLocalId != null }
            .groupBy { it.parentLocalId }
    val out = mutableListOf<Task>()
    val seen = hashSetOf(root.localId)
    val queue = ArrayDeque(listOf(root.localId))
    while (queue.isNotEmpty()) {
        for (child in byParent[queue.removeFirst()].orEmpty()) {
            if (!seen.add(child.localId)) continue
            queue.addLast(child.localId)
            if (child.status == TaskStatus.OPEN) out += child
        }
    }
    return out.filter { dayPartEdit(it, part) != TaskEdit() }
}
