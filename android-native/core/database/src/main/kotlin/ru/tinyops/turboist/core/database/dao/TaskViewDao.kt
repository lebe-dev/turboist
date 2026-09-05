package ru.tinyops.turboist.core.database.dao

import androidx.room.Dao
import androidx.room.Query
import kotlinx.coroutines.flow.Flow
import ru.tinyops.turboist.core.database.entity.TaskRow

/**
 * The sort every task list shares.
 *
 * Read it as four questions asked in order: is it pinned, how urgent is it, how
 * recently was it pinned, how recently was it created. Priority is asked before
 * pin time on purpose — a pin lifts a task above the unpinned ones, it does not
 * reorder the pinned ones among themselves — and creation time is the last word,
 * so the sort is total and a list never returns two rows in an order the engine
 * was free to choose.
 *
 * A priority spelling this build does not recognise scores nothing and sorts
 * last, which is where an unrankable task belongs: visible, at the bottom.
 *
 * This is one of the two orderings in the product. The other is the completion
 * history's, which sorts by when work finished; see the query that uses it.
 */
internal const val TASK_ORDER =
    " ORDER BY isPinned DESC, " +
        "CASE priority " +
        "WHEN 'high' THEN 4 WHEN 'medium' THEN 3 WHEN 'low' THEN 2 WHEN 'no-priority' THEN 1 " +
        "END DESC, " +
        "pinnedAt DESC, createdAt DESC"

/**
 * The optional narrowing every list accepts.
 *
 * One predicate, written once and appended everywhere, so a filter cannot mean
 * one thing on the today list and another on a project's. Binding null leaves
 * the list unnarrowed rather than matching nothing.
 */
internal const val PRIORITY_FILTER = " AND (:priority IS NULL OR priority = :priority)"

/**
 * Every statement the view queries run, named.
 *
 * They live here rather than inline on each method so that the statement a test
 * asks the engine to explain is the same string the app executes. A query plan
 * checked against a hand-copied statement proves nothing the moment the two
 * drift apart, and they always do.
 */
internal object TaskViewSql {
    const val INBOX =
        "SELECT * FROM tasks INDEXED BY index_tasks_inboxId " +
            "WHERE inboxId > 0 AND status = 'open'" + PRIORITY_FILTER + TASK_ORDER

    const val INBOX_COUNT =
        "SELECT COUNT(*) FROM tasks INDEXED BY index_tasks_inboxId " +
            "WHERE inboxId > 0 AND status = 'open'"

    const val DUE_BETWEEN =
        "SELECT * FROM tasks WHERE status = 'open' AND dueAt >= :from AND dueAt < :until" +
            PRIORITY_FILTER + TASK_ORDER

    const val DUE_BETWEEN_COUNT =
        "SELECT COUNT(*) FROM tasks WHERE status = 'open' AND dueAt >= :from AND dueAt < :until"

    const val OVERDUE =
        "SELECT * FROM tasks WHERE status = 'open' AND dueAt IS NOT NULL AND dueAt < :dayStart" +
            PRIORITY_FILTER + TASK_ORDER

    const val OVERDUE_COUNT =
        "SELECT COUNT(*) FROM tasks WHERE status = 'open' AND dueAt IS NOT NULL AND dueAt < :dayStart"

    const val WEEK =
        "WITH RECURSIVE week_tree(localId) AS (" +
            "SELECT localId FROM tasks " +
            "WHERE status = 'open' AND (planState = 'week' OR (dueAt >= :from AND dueAt < :until))" +
            PRIORITY_FILTER +
            " UNION " +
            "SELECT t.localId FROM tasks t JOIN week_tree wt ON t.parentLocalId = wt.localId " +
            "WHERE t.status = 'open'" +
            ") SELECT * FROM tasks WHERE localId IN (SELECT localId FROM week_tree)" + TASK_ORDER

    const val WEEK_PLANNED_COUNT =
        "SELECT COUNT(*) FROM tasks t WHERE t.status = 'open' AND t.planState = 'week' " +
            "AND NOT EXISTS (SELECT 1 FROM tasks p " +
            "WHERE p.localId = t.parentLocalId AND p.status = 'open' AND p.planState = 'week')"

    const val BACKLOG =
        "SELECT * FROM tasks WHERE planState = 'backlog' AND status = 'open'" + PRIORITY_FILTER + TASK_ORDER

    const val BACKLOG_COUNT = "SELECT COUNT(*) FROM tasks WHERE planState = 'backlog' AND status = 'open'"

    const val PINNED = "SELECT * FROM tasks WHERE isPinned = 1 AND status = 'open'" + PRIORITY_FILTER + TASK_ORDER

    const val PINNED_COUNT = "SELECT COUNT(*) FROM tasks WHERE isPinned = 1 AND status = 'open'"

    const val COMPLETED_BETWEEN =
        "SELECT * FROM tasks WHERE status = 'completed' " +
            "AND completedAt >= :from AND completedAt < :until" + PRIORITY_FILTER +
            " ORDER BY completedAt DESC LIMIT :limit OFFSET :offset"

    const val COMPLETED_COUNT =
        "SELECT COUNT(*) FROM tasks WHERE status = 'completed' " +
            "AND completedAt >= :from AND completedAt < :until"

    const val PROJECT = "SELECT * FROM tasks WHERE projectLocalId = :projectLocalId" + PRIORITY_FILTER + TASK_ORDER

    const val SECTION = "SELECT * FROM tasks WHERE sectionLocalId = :sectionLocalId" + PRIORITY_FILTER + TASK_ORDER

    const val CONTEXT = "SELECT * FROM tasks WHERE contextLocalId = :contextLocalId" + PRIORITY_FILTER + TASK_ORDER

    const val CONTEXT_DIRECT =
        "SELECT * FROM tasks WHERE contextLocalId = :contextLocalId AND projectLocalId IS NULL" +
            PRIORITY_FILTER + TASK_ORDER

    const val LABEL =
        "SELECT * FROM tasks WHERE localId IN (" +
            "SELECT tl.taskLocalId FROM task_labels tl WHERE tl.labelLocalId = :labelLocalId)" +
            PRIORITY_FILTER + TASK_ORDER

    const val TROIKI_CATEGORY =
        "SELECT * FROM tasks WHERE troikiCategory = :category AND status = 'open'" + TASK_ORDER

    const val TROIKI_CATEGORY_COUNT =
        "SELECT COUNT(*) FROM tasks WHERE troikiCategory = :category AND status = 'open'"

    const val TROIKI_BOARD =
        "WITH RECURSIVE cancelled_subtree(localId) AS (" +
            "SELECT localId FROM tasks WHERE status = 'cancelled'" +
            " UNION ALL " +
            "SELECT t.localId FROM tasks t " +
            "JOIN cancelled_subtree cs ON t.parentLocalId = cs.localId" +
            ") SELECT * FROM tasks WHERE projectLocalId IN (:projectLocalIds) " +
            "AND localId NOT IN (SELECT localId FROM cancelled_subtree)" + TASK_ORDER

    const val SUBTASKS = "SELECT * FROM tasks WHERE parentLocalId = :parentLocalId" + TASK_ORDER

    const val SUBTREE =
        "WITH RECURSIVE subtree(localId) AS (" +
            "SELECT localId FROM tasks WHERE parentLocalId = :parentLocalId" +
            " UNION ALL " +
            "SELECT t.localId FROM tasks t JOIN subtree s ON t.parentLocalId = s.localId" +
            ") SELECT * FROM tasks WHERE localId IN (SELECT localId FROM subtree)" + TASK_ORDER
}

/**
 * Every list the app renders, as a query against the replica.
 *
 * These are the screens. Each one returns a [Flow], so a list redraws when the
 * rows behind it change — whether they changed because the user ticked something
 * off on this device or because a sync brought the change in from another one —
 * and no screen ever has to be told to refresh itself.
 *
 * Three rules hold across all of them and are worth stating once:
 *
 * - **Windows are half-open.** Every range is `[from, until)`. The last
 *   millisecond of a day belongs to that day; midnight belongs to the next one.
 * - **Closed work is out.** Views of work to do match open tasks only. A
 *   completed or cancelled task leaves them whatever its dates or plan state
 *   still say, and is found in the completion history instead.
 * - **A list is a window, not a filing cabinet.** Membership is decided by the
 *   predicate alone: a task in an archived project is due today like any other,
 *   a private task is hidden from the shared read-only view and from nothing
 *   else, and a task that cannot be completed because something blocks it is
 *   still listed everywhere its dates put it — being blocked changes the
 *   checkbox, never the membership or the order of a list.
 *
 * The `priority` parameter of the list queries is the wire spelling of a
 * priority (`Priority.wire`), or null for an unnarrowed list.
 *
 * The orderings and predicates here are pinned against a dataset shared with the
 * server's own list queries: both sides render the same fixture into the same
 * golden orderings, so the two implementations cannot drift apart unnoticed. The
 * index each one is answered through is pinned too, by explaining the shipped
 * statements against the built schema.
 */
@Dao
abstract class TaskViewDao {
    // --- the inbox ---

    /**
     * Open tasks in the inbox. Subtasks cannot be filed there, so every row here
     * is a root.
     *
     * The index is named in the statement because this is the one list the
     * engine cannot cost for itself. It asks two questions — is this filed in
     * the inbox, is it still open — and only the second is an equality, which
     * the engine treats as the narrower of the two by default. On real data it
     * is the wider by a wide margin: nearly every task on the device is open,
     * while a handful sit in the inbox. Naming the index is what stops the list
     * reading the whole table. Room checks the name against the schema at build
     * time, so renaming the index breaks the build rather than the plan.
     *
     * The first question is asked as `inboxId > 0` rather than as a null test
     * for the same reason. An id is a positive number, so the two select the
     * same rows, but older engines — the one these tests run on among them —
     * cannot drive an index off a null test at all, and refuse the statement
     * outright once the index is named.
     */
    @Query(TaskViewSql.INBOX)
    abstract fun queryInbox(priority: String?): Flow<List<TaskRow>>

    @Query(TaskViewSql.INBOX_COUNT)
    abstract fun observeInboxCount(): Flow<Int>

    // --- dated views ---

    /**
     * Open tasks due inside a window. Both the today and the tomorrow lists are
     * this query over a different 24 hours; a task with no due date is in
     * neither, because a comparison against nothing is not a match.
     */
    @Query(TaskViewSql.DUE_BETWEEN)
    abstract fun queryDueBetween(
        from: Long,
        until: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    @Query(TaskViewSql.DUE_BETWEEN_COUNT)
    abstract fun observeDueBetweenCount(
        from: Long,
        until: Long,
    ): Flow<Int>

    /** Open tasks due on the day that begins at [dayStart]. */
    fun queryToday(
        dayStart: Long,
        priority: String? = null,
    ): Flow<List<TaskRow>> = queryDueBetween(dayStart, dayStart + DAY_MILLIS, priority)

    /** Open tasks due on the day after the one that begins at [dayStart]. */
    fun queryTomorrow(
        dayStart: Long,
        priority: String? = null,
    ): Flow<List<TaskRow>> = queryDueBetween(dayStart + DAY_MILLIS, dayStart + 2 * DAY_MILLIS, priority)

    fun observeTodayCount(dayStart: Long): Flow<Int> = observeDueBetweenCount(dayStart, dayStart + DAY_MILLIS)

    fun observeTomorrowCount(dayStart: Long): Flow<Int> =
        observeDueBetweenCount(dayStart + DAY_MILLIS, dayStart + 2 * DAY_MILLIS)

    /**
     * Open tasks whose due date has already passed. Overdue is a fact about the
     * date alone: a task parked in the backlog is overdue too, because parking
     * it said when to think about it, not when it was due.
     */
    @Query(TaskViewSql.OVERDUE)
    abstract fun queryOverdue(
        dayStart: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    @Query(TaskViewSql.OVERDUE_COUNT)
    abstract fun observeOverdueCount(dayStart: Long): Flow<Int>

    // --- the plan ---

    /**
     * The week list: open tasks the user planned for this week or that fall due
     * inside it, plus the entire open subtree under each of them, at any depth.
     *
     * The subtree is pulled in so a parent is never shown without the work it is
     * made of — a subtask with no date of its own still has to appear beneath a
     * parent that belongs to the week. Which is exactly why the number the week
     * badge shows is a different question; see [observeWeekPlannedCount].
     */
    @Query(TaskViewSql.WEEK)
    abstract fun queryWeek(
        from: Long,
        until: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    /**
     * How much of the weekly plan is spent.
     *
     * Only the tasks the user planned themselves are counted. Planning a task
     * cascades the plan state down its open subtasks, and a task that is in the
     * week solely because its parent is has not been chosen for the week — were
     * it counted, one decision would eat several slots of the limit. A task that
     * merely falls due inside the week is not counted either, for the same
     * reason: the limit is a budget of choices, not of work.
     */
    @Query(TaskViewSql.WEEK_PLANNED_COUNT)
    abstract fun observeWeekPlannedCount(): Flow<Int>

    /**
     * Open tasks parked out of the current week.
     *
     * Unlike the week list this one counts everything it shows, cascaded
     * subtasks included: the backlog is a list of what is parked, not a budget.
     */
    @Query(TaskViewSql.BACKLOG)
    abstract fun queryBacklog(priority: String?): Flow<List<TaskRow>>

    @Query(TaskViewSql.BACKLOG_COUNT)
    abstract fun observeBacklogCount(): Flow<Int>

    /** Open tasks the user pinned to the top of the workspace. */
    @Query(TaskViewSql.PINNED)
    abstract fun queryPinned(priority: String?): Flow<List<TaskRow>>

    @Query(TaskViewSql.PINNED_COUNT)
    abstract fun observePinnedCount(): Flow<Int>

    // --- the completion history ---

    /**
     * Tasks completed inside a window, most recently completed first.
     *
     * This is the one list that does not use the shared sort. The history is read
     * as days, so it has to be ordered by when work finished: ordering it by
     * priority would let an old high-priority completion push yesterday's rows
     * off the end of the page and out of the screen entirely.
     *
     * It is also the one list that is paged here rather than in the caller — the
     * history is unbounded, while every other list is bounded by the work that
     * is actually open.
     */
    @Query(TaskViewSql.COMPLETED_BETWEEN)
    abstract fun queryCompletedBetween(
        from: Long,
        until: Long,
        limit: Int,
        offset: Int,
        priority: String?,
    ): Flow<List<TaskRow>>

    @Query(TaskViewSql.COMPLETED_COUNT)
    abstract fun observeCompletedCount(
        from: Long,
        until: Long,
    ): Flow<Int>

    // --- where a task lives ---

    /** Every task of a project, whatever its status: a project page is its whole history. */
    @Query(TaskViewSql.PROJECT)
    abstract fun queryProject(
        projectLocalId: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    /** Every task filed under one board column. Always a subset of its project's list. */
    @Query(TaskViewSql.SECTION)
    abstract fun querySection(
        sectionLocalId: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    /**
     * Every task under a context, the tasks of its projects included. A task
     * inside a project carries its context too, which is what lets one query
     * answer for the whole branch.
     */
    @Query(TaskViewSql.CONTEXT)
    abstract fun queryContext(
        contextLocalId: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    /** Tasks attached to a context directly, without going through a project of it. */
    @Query(TaskViewSql.CONTEXT_DIRECT)
    abstract fun queryContextDirect(
        contextLocalId: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    /**
     * Every task carrying one label. A task carrying several labels is returned
     * once per list, not once per label it happens to match — at most one row
     * pairs a task with a label, so gathering the tasks of a label cannot
     * produce the same task twice.
     *
     * Read from the label's side rather than by asking each task whether it
     * carries the label: the tagging table is indexed by label, so this collects
     * the handful of tasks that match and looks each one up, instead of walking
     * every task on the device to ask a question about it.
     */
    @Query(TaskViewSql.LABEL)
    abstract fun queryLabel(
        labelLocalId: Long,
        priority: String?,
    ): Flow<List<TaskRow>>

    // --- the daily plan ---

    /**
     * Open tasks in one capacity bucket of the daily plan.
     *
     * Deliberately unnarrowed and unpaged: the plan draws an empty placeholder
     * for every unused slot, so the rows on screen and the count behind them have
     * to be answers to the same question.
     */
    @Query(TaskViewSql.TROIKI_CATEGORY)
    abstract fun queryTroikiCategory(category: String): Flow<List<TaskRow>>

    @Query(TaskViewSql.TROIKI_CATEGORY_COUNT)
    abstract fun observeTroikiCategoryCount(category: String): Flow<Int>

    /**
     * The tasks of the projects bound to the daily plan, for the board that
     * renders them side by side. Group them with the projects' own order.
     *
     * Completed tasks stay, so ticking one off does not make it vanish from the
     * slot it was in. Cancelled tasks go, and so does everything beneath them:
     * the board knows only "open" and "done", so a cancelled row would be drawn
     * with an empty checkbox and turn that checkbox into a "complete" button
     * against the user's decision — and an open child left behind after its
     * parent row was filtered out would be drawn as work of its own, detached
     * from the thing it was abandoned with.
     *
     * The rows of the board itself are collected through the project index. The
     * set of cancelled work it subtracts is not: there is no index that answers
     * "which tasks are cancelled" without also being the first thing every other
     * list of open work is tempted to use, and that temptation costs far more
     * than this reads. It is settled by walking the narrow two-column index over
     * completion instead, which never touches a table row.
     */
    @Query(TaskViewSql.TROIKI_BOARD)
    abstract fun queryTroikiBoard(projectLocalIds: Collection<Long>): Flow<List<TaskRow>>

    // --- subtasks ---

    /** The direct children of a task, in the shared order. */
    @Query(TaskViewSql.SUBTASKS)
    abstract fun querySubtasks(parentLocalId: Long): Flow<List<TaskRow>>

    /**
     * Every descendant of a task at any depth, flat and in the shared order. The
     * caller nests it back into a tree by the parent links it already carries.
     */
    @Query(TaskViewSql.SUBTREE)
    abstract fun querySubtree(parentLocalId: Long): Flow<List<TaskRow>>

    companion object {
        /**
         * A day, as the dated lists measure one.
         *
         * Fixed, not calendar: due dates are absolute instants and an all-day
         * task sits exactly on midnight, so a fixed span is what files it under
         * one day and only one. Weeks are measured in calendar days instead,
         * because a week has to end on a Monday midnight.
         */
        const val DAY_MILLIS: Long = 24L * 60L * 60L * 1000L
    }
}
