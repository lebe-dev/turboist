package ru.tinyops.turboist.nativeapp.projects

import ru.tinyops.turboist.core.model.ProjectSection
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.view.splitByRootCompletion
import ru.tinyops.turboist.nativeapp.tasks.TaskListRow
import ru.tinyops.turboist.nativeapp.tasks.taskListRows

/**
 * One column of a project board.
 *
 * [sectionLocalId] is `null` for the project's own column — the work that is in
 * the project but in none of its columns. That column always exists and is drawn
 * first: it is where a task lands when it is created without a column and where
 * it returns when it is taken out of one, so a board with nowhere to drop a task
 * would be a board a task could never leave.
 *
 * Finished work is kept apart from open work rather than filtered away, because
 * a project page is the project's whole history: ticking a task off on a board
 * moves it down into [done] instead of making it disappear.
 *
 * [position] is the column's place on the board, counting from the left, and is
 * `null` for the project's own column, which cannot be moved. [canMoveEarlier]
 * and [canMoveLater] say whether there is anywhere for it to go.
 */
data class BoardColumn(
    val key: String,
    val sectionLocalId: Long?,
    val title: String?,
    val position: Int?,
    val open: List<TaskListRow>,
    val done: List<TaskListRow>,
    val canMoveEarlier: Boolean = false,
    val canMoveLater: Boolean = false,
) {
    /** True once the column is known to hold nothing at all. */
    val isEmpty: Boolean get() = open.isEmpty() && done.isEmpty()
}

/**
 * Cuts a project's tasks into the columns of its board.
 *
 * The columns arrive in the order they are drawn, and the tasks in the order the
 * shared sort already decided, so nothing here re-sorts anything: this only says
 * which task belongs under which heading, and nests subtasks under the parents
 * present in the same column.
 *
 * A task naming a column this board does not have falls into the project's own
 * column rather than vanishing. That happens for a moment after a column is
 * deleted on another device — the tasks it held stay in the project, exactly as
 * they do on the server — and a row that disappeared instead would look like
 * lost work.
 */
fun boardColumns(
    sections: List<ProjectSection>,
    tasks: List<Task>,
): List<BoardColumn> {
    val known = sections.map { it.localId }.toSet()
    val byColumn = tasks.groupBy { it.sectionLocalId?.takeIf(known::contains) }
    val root =
        column(
            key = "project-root",
            sectionLocalId = null,
            title = null,
            position = null,
            tasks = byColumn[null].orEmpty(),
        )
    val columns =
        sections.mapIndexed { index, section ->
            column(
                key = "section-" + section.localId,
                sectionLocalId = section.localId,
                title = section.title,
                position = index,
                tasks = byColumn[section.localId].orEmpty(),
                canMoveEarlier = index > 0,
                canMoveLater = index < sections.size - 1,
            )
        }
    return listOf(root) + columns
}

private fun column(
    key: String,
    sectionLocalId: Long?,
    title: String?,
    position: Int?,
    tasks: List<Task>,
    canMoveEarlier: Boolean = false,
    canMoveLater: Boolean = false,
): BoardColumn {
    val split = splitByRootCompletion(tasks)
    return BoardColumn(
        key = key,
        sectionLocalId = sectionLocalId,
        title = title,
        position = position,
        // No project title on a row: every row on this screen is in the project
        // the screen is already named after.
        open = taskListRows(split.open, emptyMap()),
        done = taskListRows(split.done, emptyMap()),
        canMoveEarlier = canMoveEarlier,
        canMoveLater = canMoveLater,
    )
}
