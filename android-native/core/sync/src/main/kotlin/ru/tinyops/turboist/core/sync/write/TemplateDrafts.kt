package ru.tinyops.turboist.core.sync.write

import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.TaskRow

/**
 * Cuts a template out of work that already exists.
 *
 * The draft is built from the replica rather than asked for, which is the whole
 * point: the gesture is "make this repeatable", and it has to work on a task
 * written down an hour ago that the server has never heard of, on a train. What
 * comes back is only a draft — nothing is written anywhere until it is saved
 * through [TemplateWriteRepo.create], which is also what the editor it prefills
 * expects.
 *
 * A template is one level deep, so the whole subtree below the task is flattened
 * into the checklist in reading order: each task, then everything under it,
 * before its next sibling. Anything else would silently drop the work nested
 * deeper, or reorder it against the tree the user is looking at.
 */
class TemplateDrafts(private val db: TurboistDatabase) {
    /**
     * The template a task and its subtree would make.
     *
     * The task's title becomes the template's name, since that is what a task
     * created from it is called. Labels travel as this device's own ids, which is
     * what the editor and the queued write both speak.
     *
     * @throws WriteRefused.RowMissing when the task is not in the replica.
     */
    suspend fun fromTask(taskLocalId: Long): TemplateDraft {
        val root =
            db.tasks().byLocalId(taskLocalId)
                ?: throw WriteRefused.RowMissing("task", taskLocalId)
        return TemplateDraft(
            name = root.title,
            description = root.description,
            priority = root.priority,
            dayPart = root.dayPart,
            labelLocalIds = labelLocalIds(root.localId),
            subtasks =
                flatten(root.localId).map { line ->
                    TemplateSubtaskDraft(
                        title = line.title,
                        description = line.description,
                        priority = line.priority,
                        dayPart = line.dayPart,
                        labelLocalIds = labelLocalIds(line.localId),
                    )
                },
        )
    }

    /**
     * Everything below a task, in reading order.
     *
     * Walked with a stack rather than by recursion, and with the tasks already
     * visited remembered: these rows were copied from a server, and a replica
     * somehow left with a loop in its parent links must still answer rather than
     * spin.
     */
    private suspend fun flatten(taskLocalId: Long): List<TaskRow> {
        val out = mutableListOf<TaskRow>()
        val seen = mutableSetOf(taskLocalId)
        val pending = ArrayDeque(db.tasks().subtasksOf(taskLocalId))
        while (pending.isNotEmpty()) {
            val next = pending.removeFirst()
            if (!seen.add(next.localId)) continue
            out += next
            // The children go in front of the remaining siblings, which is what
            // makes the order "this task, then its own work, then the next one".
            pending.addAll(0, db.tasks().subtasksOf(next.localId))
        }
        return out
    }

    private suspend fun labelLocalIds(taskLocalId: Long): List<Long> =
        db.tasks().labelsOf(taskLocalId).map { it.labelLocalId }
}
