package ru.tinyops.turboist.core.database.view

import android.database.Cursor
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Test
import ru.tinyops.turboist.core.database.ReplicaTest
import ru.tinyops.turboist.core.database.dao.TaskViewSql
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.TroikiCategory
import ru.tinyops.turboist.core.model.view.groupByProject
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The lists, against the contract they share with the server.
 *
 * One dataset is written into the replica, every list is rendered from it, and
 * the result is compared with the orderings stored beside that dataset — the
 * same orderings the server's own list queries are required to produce from the
 * same rows. Two implementations of the same rules will always drift apart on
 * their own; this is what makes the drift a failing build instead of a screen
 * that quietly disagrees with the web app.
 *
 * When a list here stops matching, exactly one of two things is true: the query
 * is wrong, or a rule genuinely changed. A rule that genuinely changed is
 * changed in the shared dataset first, on both sides at once. Nothing about a
 * list is ever "adjusted" here alone.
 */
class ViewContractTest : ReplicaTest() {
    @Test
    fun `every list renders exactly the ordering the shared contract pins`() =
        runBlocking {
            val fixture = loadContractFixture(db)
            val failures = mutableListOf<String>()
            for (view in fixture.declaredViews) {
                val golden = ContractData.golden(view)
                if (golden["groups"] != null) {
                    val rendered = renderGrouped(view, fixture)
                    val expected = golden.groups()
                    if (rendered != expected) {
                        failures += "$view\n  expected ${expected}\n  rendered $rendered"
                    }
                    continue
                }
                val rendered = render(view, fixture)
                val expectedKeys = golden.taskKeys()
                val expectedTotal = golden.total()
                if (rendered.taskKeys != expectedKeys) {
                    failures += "$view\n  expected ${expectedKeys}\n  rendered ${rendered.taskKeys}"
                }
                if (rendered.total != expectedTotal) {
                    failures += "$view total\n  expected $expectedTotal\n  rendered ${rendered.total}"
                }
            }
            assertTrue(
                failures.isEmpty(),
                "these lists no longer match the shared contract:\n" + failures.joinToString("\n"),
            )
        }

    @Test
    fun `the dataset declares every list that is rendered from it`() =
        runBlocking {
            val fixture = loadContractFixture(db)
            // A list that renders but is not declared has no expected ordering
            // and would pass by never being looked at.
            assertEquals(
                fixture.declaredViews.toSet(),
                RENDERED_VIEWS,
                "the shared dataset and the lists rendered from it name different sets",
            )
        }

    @Test
    fun `the counters behind the drawer badges agree with the lists they summarise`() =
        runBlocking {
            val fixture = loadContractFixture(db)
            val views = db.taskViews()
            val todayStart = fixture.instant("todayStart")

            // Each counter answers the same question as its list, so it must
            // return the number the list's own contract states — a badge that
            // disagrees with the screen behind it is worse than no badge.
            assertEquals(ContractData.golden("inbox").total(), views.observeInboxCount().value())
            assertEquals(ContractData.golden("today").total(), views.observeTodayCount(todayStart).value())
            assertEquals(ContractData.golden("tomorrow").total(), views.observeTomorrowCount(todayStart).value())
            assertEquals(ContractData.golden("overdue").total(), views.observeOverdueCount(todayStart).value())
            assertEquals(ContractData.golden("backlog").total(), views.observeBacklogCount().value())
            assertEquals(ContractData.golden("pinned").total(), views.observePinnedCount().value())
            assertEquals(ContractData.golden("week").total(), views.observeWeekPlannedCount().value())
            for (category in TROIKI_CATEGORIES) {
                assertEquals(
                    ContractData.golden("troiki-${category.wire}").total(),
                    views.observeTroikiCategoryCount(category.wire).value(),
                    "the ${category.wire} slot counter",
                )
            }
        }

    @Test
    fun `the week badge counts choices rather than rows`() =
        runBlocking {
            val fixture = loadContractFixture(db)
            val rendered = render("week", fixture)
            // Planning a task pulls its whole open subtree into the week list,
            // and a task merely due this week is listed too. Neither spends a
            // slot of the weekly limit, so the badge is far smaller than the
            // list — and it has to stay that way.
            assertTrue(
                rendered.total < rendered.taskKeys.size,
                "the week list is built from more than the tasks the user chose for it",
            )
        }

    @Test
    fun `every list drives off the index it was given`() {
        // Not "an index exists on the column" — which index the engine actually
        // picks. It has no statistics about the rows on a device, so it costs an
        // equality test as narrow and a null test as wide, and on this schema
        // that is backwards: nearly every task is open, while a day, a project
        // or the inbox holds a handful. One index added for one list can
        // therefore quietly move three other lists onto a read of every open
        // task, which is exactly what "indexed" alone would never notice.
        val failures = mutableListOf<String>()
        for ((statement, sql) in viewStatements()) {
            val plan = planOf(sql)
            for (index in REQUIRED_INDEX.getValue(statement)) {
                val searched = plan.any { it.startsWith("SEARCH") && it.contains(" $index") }
                if (!searched) {
                    failures += "$statement no longer searches $index\n  " + plan.joinToString("\n  ")
                }
            }
        }
        assertTrue(failures.isEmpty(), "the engine changed its mind about these lists:\n" + failures.joinToString("\n"))
    }

    @Test
    fun `every statement the views run has its plan pinned`() {
        // A query added without a line here would go unwatched, which is the
        // one way the rule above stops holding.
        assertEquals(
            viewStatements().keys,
            REQUIRED_INDEX.keys,
            "a view statement was added or removed without pinning the index it is answered through",
        )
    }

    @Test
    fun `no list reads the task table row by row`() {
        val failures = mutableListOf<String>()
        for ((statement, sql) in viewStatements()) {
            for (step in planOf(sql)) {
                // A step over rows a recursive query built for itself is not a
                // read of the stored table; everything else has to name an
                // index it was answered through.
                val subject = subjectOfScan(step) ?: continue
                if (subject in DERIVED_ROWS) continue
                if (INDEXED_READ.none { step.contains(it) }) failures += "$statement: $step"
            }
        }
        assertTrue(
            failures.isEmpty(),
            "these steps read a table instead of an index:\n" + failures.joinToString("\n"),
        )
    }

    @Test
    fun `a label list returns a task once however many labels it carries`() =
        runBlocking {
            val fixture = loadContractFixture(db)
            val keys = render("label-urgent", fixture).taskKeys
            assertEquals(keys.distinct(), keys, "a task carrying several labels was listed once per label")
        }

    // --- rendering ---

    private data class RenderedView(
        val taskKeys: List<String>,
        val total: Int,
    )

    private suspend fun render(
        view: String,
        fixture: LoadedFixture,
    ): RenderedView {
        val views = db.taskViews()
        val todayStart = fixture.instant("todayStart")
        return when (view) {
            "inbox" -> views.queryInbox(null).rendered(fixture, views.observeInboxCount())
            "today" -> views.queryToday(todayStart).rendered(fixture, views.observeTodayCount(todayStart))
            "today-high-priority" -> views.queryToday(todayStart, Priority.HIGH.wire).rendered(fixture)
            "tomorrow" -> views.queryTomorrow(todayStart).rendered(fixture, views.observeTomorrowCount(todayStart))
            "overdue" -> views.queryOverdue(todayStart, null).rendered(fixture, views.observeOverdueCount(todayStart))
            "week" ->
                views
                    .queryWeek(fixture.instant("weekStart"), fixture.instant("weekEnd"), null)
                    .rendered(fixture, views.observeWeekPlannedCount())

            "backlog" -> views.queryBacklog(null).rendered(fixture, views.observeBacklogCount())
            "pinned" -> views.queryPinned(null).rendered(fixture, views.observePinnedCount())
            "completed" -> {
                val from = fixture.instant("completedWindowStart")
                val until = fixture.instant("completedWindowEnd")
                views
                    .queryCompletedBetween(from, until, HISTORY_PAGE, 0, null)
                    .rendered(fixture, views.observeCompletedCount(from, until))
            }

            "troiki-important", "troiki-medium", "troiki-rest" -> {
                val wire = view.removePrefix("troiki-")
                views.queryTroikiCategory(wire).rendered(fixture, views.observeTroikiCategoryCount(wire))
            }

            "project-alpha" -> views.queryProject(fixture.projects.getValue("prj-alpha"), null).rendered(fixture)
            "section-alpha-doing" ->
                views.querySection(fixture.sections.getValue("sec-alpha-doing"), null).rendered(fixture)

            "label-urgent" -> views.queryLabel(fixture.labels.getValue("lbl-urgent"), null).rendered(fixture)
            "context-work-direct" ->
                views.queryContextDirect(fixture.contexts.getValue("ctx-work"), null).rendered(fixture)

            "context-work-all" -> views.queryContext(fixture.contexts.getValue("ctx-work"), null).rendered(fixture)
            "subtasks-direct" -> views.querySubtasks(fixture.tasks.getValue(SUBTASK_PARENT)).rendered(fixture)
            "subtasks-recursive" -> views.querySubtree(fixture.tasks.getValue(SUBTASK_PARENT)).rendered(fixture)
            else -> error("the shared dataset declares a list this test cannot render: $view")
        }
    }

    private suspend fun renderGrouped(
        view: String,
        fixture: LoadedFixture,
    ): List<Pair<String, List<String>>> {
        check(view == "troiki-board") { "the shared dataset declares a grouped list this test cannot render: $view" }
        val projectKeys = BOARD_PROJECTS
        val projectLocalIds = projectKeys.map { fixture.projects.getValue(it) }
        val rows = db.taskViews().queryTroikiBoard(projectLocalIds).value()
        return groupByProject(rows.map { it.toDomain() }, projectLocalIds)
            .mapIndexed { index, group ->
                projectKeys[index] to group.tasks.map { fixture.keyOf(it.localId) }
            }
    }

    private suspend fun Flow<List<TaskRow>>.rendered(
        fixture: LoadedFixture,
        count: Flow<Int>? = null,
    ): RenderedView {
        val rows = value()
        val keys = fixture.keysOf(rows)
        return RenderedView(keys, count?.value() ?: keys.size)
    }

    private suspend fun <T> Flow<T>.value(): T = first()

    /**
     * Every statement the view queries run, read off the object that holds
     * them, so a plan is checked against the string the app executes rather
     * than against a copy of it that can drift.
     */
    private fun viewStatements(): Map<String, String> =
        TaskViewSql::class.java.declaredFields
            .filter { it.type == String::class.java }
            .associate { it.name to (it.get(null) as String) }

    /**
     * What a step of a plan reads, if it reads it whole.
     *
     * Null for a step that is not a scan at all, and for a scan of rows the
     * query produced for itself rather than of something stored. Engines word
     * a plan differently between versions — a stored table is named on its own
     * in one and after the word `TABLE` in another — so the wording is taken
     * apart rather than matched.
     */
    private fun subjectOfScan(step: String): String? {
        val words = step.split(' ')
        if (words.firstOrNull() != "SCAN") return null
        val subject = words.drop(1).firstOrNull { it != "TABLE" } ?: return null
        return subject.takeUnless { it == "SUBQUERY" || it == "CONSTANT" }
    }

    /** How the engine says it would answer a statement, one step per line. */
    private fun planOf(sql: String): List<String> =
        query("EXPLAIN QUERY PLAN $sql") { it.getString(it.columnCount - 1) }

    private fun <T> query(
        sql: String,
        read: (Cursor) -> T,
    ): List<T> {
        val results = mutableListOf<T>()
        db.openHelper.readableDatabase.query(sql).use { cursor ->
            while (cursor.moveToNext()) results += read(cursor)
        }
        return results
    }

    private companion object {
        /**
         * The index each statement has to be answered through.
         *
         * A list of a project, a section, a context, a label or a parent is
         * collected through that thing's own index. A dated list and the
         * completion history are collected through the two-column index that
         * leads with the date. The workspace-wide lists — the backlog, the
         * pinned list, the daily plan, the inbox — are collected through the
         * column that is empty on most rows. The week list is three of those at
         * once: the planned tasks, the tasks falling due, and the subtree walk
         * beneath both.
         */
        val REQUIRED_INDEX =
            mapOf(
                "INBOX" to listOf("index_tasks_inboxId"),
                "INBOX_COUNT" to listOf("index_tasks_inboxId"),
                "DUE_BETWEEN" to listOf("index_tasks_dueAt_status"),
                "DUE_BETWEEN_COUNT" to listOf("index_tasks_dueAt_status"),
                "OVERDUE" to listOf("index_tasks_dueAt_status"),
                "OVERDUE_COUNT" to listOf("index_tasks_dueAt_status"),
                "WEEK" to
                    listOf(
                        "index_tasks_planState",
                        "index_tasks_dueAt_status",
                        "index_tasks_parentLocalId",
                    ),
                "WEEK_PLANNED_COUNT" to listOf("index_tasks_planState"),
                "BACKLOG" to listOf("index_tasks_planState"),
                "BACKLOG_COUNT" to listOf("index_tasks_planState"),
                "PINNED" to listOf("index_tasks_isPinned"),
                "PINNED_COUNT" to listOf("index_tasks_isPinned"),
                "COMPLETED_BETWEEN" to listOf("index_tasks_completedAt_status"),
                "COMPLETED_COUNT" to listOf("index_tasks_completedAt_status"),
                "PROJECT" to listOf("index_tasks_projectLocalId"),
                "SECTION" to listOf("index_tasks_sectionLocalId"),
                "CONTEXT" to listOf("index_tasks_contextLocalId"),
                "CONTEXT_DIRECT" to listOf("index_tasks_contextLocalId"),
                "LABEL" to listOf("index_task_labels_labelLocalId"),
                "TROIKI_CATEGORY" to listOf("index_tasks_troikiCategory"),
                "TROIKI_CATEGORY_COUNT" to listOf("index_tasks_troikiCategory"),
                "TROIKI_BOARD" to listOf("index_tasks_projectLocalId"),
                "SUBTASKS" to listOf("index_tasks_parentLocalId"),
                "SUBTREE" to listOf("index_tasks_parentLocalId"),
            )

        /** The rows a recursive query builds for itself, and the aliases they are read under. */
        val DERIVED_ROWS = setOf("week_tree", "wt", "subtree", "s", "cancelled_subtree", "cs")

        /** How the engine says a step read an index rather than the table behind it. */
        val INDEXED_READ = listOf("USING INDEX", "USING COVERING INDEX", "USING INTEGER PRIMARY KEY")

        /**
         * Large enough that the completion history is rendered whole, so the
         * comparison is against the full ordering rather than against whatever
         * the first page happened to hold.
         */
        const val HISTORY_PAGE = 200

        const val SUBTASK_PARENT = "t-week-planned-high"

        val TROIKI_CATEGORIES = listOf(TroikiCategory.IMPORTANT, TroikiCategory.MEDIUM, TroikiCategory.REST)

        val BOARD_PROJECTS = listOf("prj-alpha", "prj-beta", "prj-home")

        val RENDERED_VIEWS =
            setOf(
                "inbox",
                "today",
                "today-high-priority",
                "tomorrow",
                "overdue",
                "week",
                "backlog",
                "pinned",
                "completed",
                "troiki-important",
                "troiki-medium",
                "troiki-rest",
                "troiki-board",
                "project-alpha",
                "section-alpha-doing",
                "label-urgent",
                "context-work-direct",
                "context-work-all",
                "subtasks-direct",
                "subtasks-recursive",
            )
    }
}

private fun JsonObject.taskKeys(): List<String> =
    (this["taskKeys"] as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.content } ?: emptyList()

private fun JsonObject.total(): Int =
    (this["total"] as? JsonPrimitive)?.content?.toInt() ?: error("the stored ordering states no total")

private fun JsonObject.groups(): List<Pair<String, List<String>>> =
    (this["groups"] as? JsonArray)
        ?.filterIsInstance<JsonObject>()
        ?.map { group ->
            val key = (group["projectKey"] as? JsonPrimitive)?.content ?: error("a stored group names no project")
            key to group.taskKeys()
        }
        ?: emptyList()
