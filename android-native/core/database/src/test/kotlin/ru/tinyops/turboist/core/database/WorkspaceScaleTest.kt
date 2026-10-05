package ru.tinyops.turboist.core.database

import android.database.Cursor
import androidx.room.withTransaction
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import org.junit.Test
import ru.tinyops.turboist.core.database.dao.TaskViewSql
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.TaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.search.FtsQuery
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.TroikiCategory
import kotlin.math.max
import kotlin.system.measureNanoTime
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The replica at the size a long-used workspace reaches.
 *
 * The lists are already held to the index each one is answered through, which
 * settles the *shape* of a query. This settles its *cost*, which is a different
 * question and the one a user feels: a list that seeks costs a fraction of a
 * read of the task table, and a list that has quietly started reading every row
 * costs about the same as one. So every statement is timed against a deliberate
 * full read of the same table, taken on the same machine in the same run — a
 * ratio rather than a millisecond count, because a millisecond count measures
 * whichever machine happened to run the build and would be re-tuned until it
 * passed instead of being believed.
 *
 * The numbers themselves are printed, because a ratio says a list is still
 * cheap and says nothing about how cheap. The recorded baselines live beside the
 * module's own notes; this is what regenerates them.
 */
class WorkspaceScaleTest : ReplicaTest() {
    /**
     * The seeded workspace, sized past what a heavy user reaches: a task list
     * that stays cheap here stays cheap on a real device, where the phone is
     * slower but the workspace is smaller.
     */
    private companion object {
        const val TASKS = 10_000
        const val PROJECTS = 200
        const val CONTEXTS = 5
        const val LABELS = 20
        const val SECTIONED_PROJECTS = 20
        const val SECTIONS_PER_PROJECT = 3

        /** How deep the deepest chain of subtasks goes. */
        const val SUBTASK_DEPTH = 12

        /** Tasks that hang directly under a parent, on top of the deep chain. */
        const val FLAT_SUBTASKS = 400

        const val INBOX_TASKS = 300
        const val DUE_TODAY = 60
        const val DUE_TOMORROW = 40
        const val OVERDUE = 120
        const val DUE_LATER_IN_WEEK = 500
        const val PLANNED_FOR_WEEK = 150
        const val BACKLOG = 800
        const val PINNED = 10
        const val PER_TROIKI_CATEGORY = 3
        const val COMPLETED = 3_000
        const val RELATIONS = 300

        /**
         * The three buckets of the daily plan. The enum's fourth entry only
         * stands for a spelling this build does not know.
         */
        val TROIKI_BUCKETS = listOf(TroikiCategory.IMPORTANT, TroikiCategory.MEDIUM, TroikiCategory.REST)

        /** The priorities a task is actually written with, in the order the shared sort ranks them. */
        val PRIORITIES = listOf(Priority.NONE, Priority.LOW, Priority.MEDIUM, Priority.HIGH)

        /** The phases a dated task can be filed under. */
        val DAY_PARTS = listOf(DayPart.NONE, DayPart.MORNING, DayPart.AFTERNOON, DayPart.EVENING)

        /** The fixed inbox row, which is the same number on every device. */
        const val INBOX_ID = 2L

        const val DAY_MILLIS = 24L * 60L * 60L * 1000L

        /** Midnight of the seeded day, so every dated list has a window to ask about. */
        const val TODAY_START = 1_709_942_400_000L

        /**
         * The share of a full read of the task table a list may cost.
         *
         * Deliberately loose. The lists that must seek — a day, the inbox, a
         * project — touch tens of rows against ten thousand and land three
         * orders of magnitude below this, so the slack never decides their
         * verdict; it is there for the one list that legitimately holds a fifth
         * of the workspace, everything under a context. What the line divides is
         * seeking from scanning, and a list that has started reading the whole
         * table costs a full read plus the sort on top of it — well over this
         * rather than near it. A tighter line would be re-tuned whenever a build
         * machine was busy instead of being believed.
         */
        const val COST_BUDGET = 0.5

        /** Times taken per statement; the middle one is reported, so noise is dropped. */
        const val SAMPLES = 5
    }

    private lateinit var seeded: SeededWorkspace

    /** What the seeding produced, so the queries can be asked about real rows. */
    private data class SeededWorkspace(
        val projectLocalIds: List<Long>,
        val sectionLocalId: Long,
        val contextLocalId: Long,
        val labelLocalId: Long,
        val deepestParentLocalId: Long,
        val wideParentLocalId: Long,
        val subtasks: Int,
        val taggings: Int,
    )

    @Test
    fun `every list stays a seek over a workspace of ten thousand tasks`() =
        runBlocking {
            seeded = seedWorkspace()

            val fullRead = cost("a full read of the task table", "SELECT * FROM tasks")
            val measurements = viewStatements().map { (name, sql) -> cost(name, bind(name, sql)) }

            report("list queries", fullRead, measurements)

            val overBudget =
                measurements.filter { it.nanos > fullRead.nanos * COST_BUDGET }
                    .map { "${it.name} cost ${it.share(fullRead)} of a full read of the table" }
            assertTrue(
                overBudget.isEmpty(),
                "these lists no longer seek — they read about as much as the whole table:\n" +
                    overBudget.joinToString("\n"),
            )
        }

    @Test
    fun `the dated lists answer with a day rather than with the workspace`() =
        runBlocking {
            seeded = seedWorkspace()
            val views = db.taskViews()

            // The point of the seeded sizes: each list is a fixed handful out of
            // ten thousand rows, so a query that started returning the workspace
            // would fail here rather than merely be slow.
            assertEquals(DUE_TODAY, views.queryToday(TODAY_START).rows())
            assertEquals(DUE_TOMORROW, views.queryTomorrow(TODAY_START).rows())
            assertEquals(OVERDUE, views.queryOverdue(TODAY_START, null).rows())
            assertEquals(INBOX_TASKS, views.queryInbox(null).rows())
            assertEquals(PINNED, views.queryPinned(null).rows())
            assertEquals(BACKLOG, views.queryBacklog(null).rows())
        }

    @Test
    fun `the rows a list is hydrated with grow with what carries them, not with the workspace`() =
        runBlocking {
            seeded = seedWorkspace()
            val hydration = db.taskHydration()

            // Every list is drawn with three standing queries beside it, and each
            // one is fetched whole. That is only affordable while each stays
            // proportional to the handful of rows that have something to say —
            // the tasks that carry a label, the tasks that are subtasks, the
            // relation edges — rather than to the task table. A query here that
            // started answering per task would be re-run and re-joined on every
            // single write the replica takes.
            val taggings = hydration.observeTaskLabels().first().size
            val parentLinks = hydration.observeParentLinks().first().size
            val edges = hydration.observeRelationEdges().first().size

            assertEquals(seeded.taggings, taggings)
            assertEquals(seeded.subtasks, parentLinks)
            assertEquals(RELATIONS, edges)
            assertTrue(
                parentLinks < TASKS / 4 && edges < TASKS / 4,
                "a standing query beside every list has grown to the size of the workspace: " +
                    "$parentLinks parent links and $edges relation edges against $TASKS tasks",
            )

            report(
                "hydration queries",
                cost("a full read of the task table", "SELECT * FROM tasks"),
                listOf(
                    cost("taskLabels", HYDRATION_LABELS),
                    cost("parentLinks", HYDRATION_PARENTS),
                    cost("relationEdges", HYDRATION_EDGES),
                ),
            )
        }

    @Test
    fun `a search over ten thousand tasks is answered out of the index`() =
        runBlocking {
            seeded = seedWorkspace()

            val found =
                db.search().tasks(
                    query = requireNotNull(FtsQuery.match("passport")),
                    titleQuery = requireNotNull(FtsQuery.matchIn(FtsQuery.TITLE_COLUMN, "passport")),
                    status = null,
                    limit = 50,
                )

            assertEquals(listOf("Renew passport", "Book flights"), found.map { it.title })

            val fullRead = cost("a full read of the task table", "SELECT * FROM tasks")
            val search =
                cost(
                    "search",
                    "SELECT * FROM tasks WHERE localId IN " +
                        "(SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH 'passport') " +
                        "ORDER BY updatedAt DESC LIMIT 50",
                )
            report("search", fullRead, listOf(search))
            assertTrue(
                search.nanos <= fullRead.nanos * COST_BUDGET,
                "a search is being answered by reading the workspace instead of the index: " +
                    search.share(fullRead) + " of a full read of the table",
            )
        }

    @Test
    fun `a deep chain of subtasks is walked to its end`() =
        runBlocking {
            seeded = seedWorkspace()

            val subtree = db.taskViews().querySubtree(seeded.deepestParentLocalId).first()

            // Every level below the root, and nothing from the rest of the
            // workspace: the walk follows parent links rather than filtering the
            // table, so its cost is the depth and not the row count.
            assertEquals(SUBTASK_DEPTH, subtree.size)
        }

    // --- measuring ---

    /** One statement and what it cost, in the middle of several attempts. */
    private data class Measurement(
        val name: String,
        val nanos: Long,
        val rows: Int,
    ) {
        fun share(baseline: Measurement): String =
            String.format("%.2f%%", 100.0 * nanos.toDouble() / max(1L, baseline.nanos).toDouble())
    }

    /**
     * Runs a statement until its cost settles, and reports the middle reading.
     *
     * The first run of a statement pays for parsing and for pages the engine has
     * not touched yet, neither of which a screen pays on a device that has been
     * open for a second. Taking the median of the runs afterwards drops the one
     * that happened to land on a busy moment of the build machine.
     */
    private fun cost(
        name: String,
        sql: String,
    ): Measurement {
        readAll(sql)
        val readings = mutableListOf<Long>()
        var rows = 0
        repeat(SAMPLES) {
            readings += measureNanoTime { rows = readAll(sql) }
        }
        return Measurement(name, readings.sorted()[SAMPLES / 2], rows)
    }

    /** Reads every row and every column of a statement, as a screen would. */
    private fun readAll(sql: String): Int {
        var rows = 0
        db.openHelper.readableDatabase.query(sql).use { cursor ->
            while (cursor.moveToNext()) {
                readRow(cursor)
                rows++
            }
        }
        return rows
    }

    private fun readRow(cursor: Cursor) {
        for (column in 0 until cursor.columnCount) {
            when (cursor.getType(column)) {
                Cursor.FIELD_TYPE_INTEGER -> cursor.getLong(column)
                Cursor.FIELD_TYPE_FLOAT -> cursor.getDouble(column)
                Cursor.FIELD_TYPE_STRING -> cursor.getString(column)
                else -> Unit
            }
        }
    }

    private fun report(
        heading: String,
        baseline: Measurement,
        measurements: List<Measurement>,
    ) {
        val lines =
            (listOf(baseline) + measurements.sortedByDescending { it.nanos }).joinToString("\n") {
                String.format(
                    "  %-24s %8.3f ms  %6d rows  %8s of a full read",
                    it.name,
                    it.nanos / 1_000_000.0,
                    it.rows,
                    it.share(baseline),
                )
            }
        println("$heading over $TASKS tasks, $PROJECTS projects:\n$lines")
    }

    /**
     * Every statement the lists run, read off the object that holds them, so a
     * cost is measured against the string the app executes.
     */
    private fun viewStatements(): Map<String, String> =
        TaskViewSql::class.java.declaredFields
            .filter { it.type == String::class.java }
            .associate { it.name to (it.get(null) as String) }

    /**
     * Fills a statement's parameters in with the rows the seeding produced.
     *
     * The engine takes the values inline rather than bound, because a bound
     * statement cannot be handed to it as text — and because the plan it picks is
     * already pinned elsewhere, so nothing here depends on the difference.
     * The longest names are replaced first, so one parameter cannot be replaced
     * inside another's name.
     */
    private fun bind(
        name: String,
        sql: String,
    ): String {
        val values =
            mapOf(
                "priority" to "NULL",
                "from" to TODAY_START.toString(),
                "until" to (TODAY_START + 7 * DAY_MILLIS).toString(),
                "dayStart" to TODAY_START.toString(),
                "limit" to "50",
                "offset" to "0",
                "projectLocalIds" to seeded.projectLocalIds.take(3).joinToString(","),
                "projectLocalId" to seeded.projectLocalIds.first().toString(),
                "sectionLocalId" to seeded.sectionLocalId.toString(),
                "contextLocalId" to seeded.contextLocalId.toString(),
                "labelLocalId" to seeded.labelLocalId.toString(),
                // The parent with a real fan-out under it, so the two subtask
                // statements are measured against work rather than against one row.
                "parentLocalId" to seeded.wideParentLocalId.toString(),
                "category" to "'${TroikiCategory.IMPORTANT.wire}'",
            )
        val bound =
            values.entries
                .sortedByDescending { it.key.length }
                .fold(sql) { statement, (parameter, value) -> statement.replace(":$parameter", value) }
        check(!bound.contains(':')) { "$name has a parameter this test does not supply a value for: $bound" }
        return bound
    }

    private suspend fun Flow<List<TaskRow>>.rows(): Int = first().size

    // --- seeding ---

    /**
     * Writes a workspace of the stated size.
     *
     * One transaction, because ten thousand statements each committing on their
     * own is a test that spends its time on the disk rather than on the question
     * it is asking.
     */
    @Suppress("LongMethod", "CyclomaticComplexMethod")
    private suspend fun seedWorkspace(): SeededWorkspace {
        val contextIds = mutableListOf<Long>()
        val projectIds = mutableListOf<Long>()
        val labelIds = mutableListOf<Long>()
        var sectionId = 0L
        var subtasks = 0
        var taggings = 0
        var deepestParent = 0L
        var wideParent = 0L

        db.withTransaction {
            repeat(CONTEXTS) { index ->
                contextIds +=
                    db.contexts().insert(ContextRow(name = "Context $index", createdAt = NOW, updatedAt = NOW))
            }
            repeat(LABELS) { index ->
                labelIds += db.labels().insert(LabelRow(name = "label-$index", createdAt = NOW, updatedAt = NOW))
            }
            repeat(PROJECTS) { index ->
                val projectId =
                    db.projects().insert(
                        ProjectRow(
                            contextLocalId = contextIds[index % CONTEXTS],
                            title = "Project $index",
                            createdAt = NOW,
                            updatedAt = NOW,
                        ),
                    )
                projectIds += projectId
                if (index < SECTIONED_PROJECTS) {
                    repeat(SECTIONS_PER_PROJECT) { column ->
                        val id =
                            db.sections().insert(
                                ProjectSectionRow(
                                    projectLocalId = projectId,
                                    title = "Column $column",
                                    position = column,
                                    createdAt = NOW,
                                    updatedAt = NOW,
                                ),
                            )
                        if (sectionId == 0L) sectionId = id
                    }
                }
            }

            var written = 0

            fun placement(index: Int): Long = projectIds[index % PROJECTS]

            // The dated work, the planned work and the workspace-wide shelves,
            // each in the fixed number the assertions above count on.
            repeat(INBOX_TASKS) { index ->
                db.tasks().insert(seedTask("Inbox note $index", inboxId = INBOX_ID))
                written++
            }
            repeat(DUE_TODAY) { index ->
                db.tasks().insert(
                    seedTask(
                        "Due today $index",
                        projectLocalId = placement(written),
                        dueAt = TODAY_START + index * 1_000L,
                        dayPart = DAY_PARTS[index % DAY_PARTS.size],
                    ),
                )
                written++
            }
            repeat(DUE_TOMORROW) { index ->
                db.tasks().insert(
                    seedTask(
                        "Due tomorrow $index",
                        projectLocalId = placement(written),
                        dueAt = TODAY_START + DAY_MILLIS,
                    ),
                )
                written++
            }
            repeat(OVERDUE) { index ->
                db.tasks().insert(
                    seedTask(
                        "Overdue $index",
                        projectLocalId = placement(written),
                        dueAt = TODAY_START - (index + 1) * 1_000L,
                    ),
                )
                written++
            }
            repeat(DUE_LATER_IN_WEEK) { index ->
                db.tasks().insert(
                    seedTask(
                        "Later this week $index",
                        projectLocalId = placement(written),
                        dueAt = TODAY_START + 2 * DAY_MILLIS + index * 1_000L,
                    ),
                )
                written++
            }
            repeat(PLANNED_FOR_WEEK) { index ->
                db.tasks().insert(
                    seedTask("Planned $index", projectLocalId = placement(written), planState = PlanState.WEEK),
                )
                written++
            }
            repeat(BACKLOG) { index ->
                db.tasks().insert(
                    seedTask("Parked $index", projectLocalId = placement(written), planState = PlanState.BACKLOG),
                )
                written++
            }
            repeat(PINNED) { index ->
                db.tasks().insert(
                    seedTask("Pinned $index", projectLocalId = placement(written), pinned = true),
                )
                written++
            }
            for (category in TROIKI_BUCKETS) {
                repeat(PER_TROIKI_CATEGORY) { index ->
                    db.tasks().insert(
                        seedTask(
                            "Plan slot ${category.wire} $index",
                            projectLocalId = placement(written),
                            troikiCategory = category,
                        ),
                    )
                    written++
                }
            }
            repeat(COMPLETED) { index ->
                db.tasks().insert(
                    seedTask(
                        "Finished $index",
                        projectLocalId = placement(written),
                        status = TaskStatus.COMPLETED,
                        completedAt = TODAY_START - (index % 90) * DAY_MILLIS,
                    ),
                )
                written++
            }

            // One chain as deep as a task ever nests, and a spread of ordinary
            // one-level subtasks beside it.
            var parent = db.tasks().insert(seedTask("Deep root", projectLocalId = projectIds.first()))
            written++
            deepestParent = parent
            repeat(SUBTASK_DEPTH) { depth ->
                parent =
                    db.tasks().insert(
                        seedTask("Deep step $depth", projectLocalId = projectIds.first(), parentLocalId = parent),
                    )
                written++
                subtasks++
            }
            wideParent = db.tasks().insert(seedTask("Wide root", projectLocalId = projectIds.first()))
            written++
            repeat(FLAT_SUBTASKS) { index ->
                db.tasks().insert(
                    seedTask(
                        "Subtask $index",
                        projectLocalId = projectIds.first(),
                        parentLocalId = wideParent,
                    ),
                )
                written++
                subtasks++
            }

            // The two rows the search case looks for, and then plain work up to
            // the stated size.
            db.tasks().insert(seedTask("Renew passport", contextLocalId = contextIds.first()))
            db.tasks().insert(
                seedTask("Book flights", contextLocalId = contextIds.first(), description = "passport first"),
            )
            written += 2
            while (written < TASKS) {
                db.tasks().insert(
                    seedTask(
                        "Task $written",
                        projectLocalId = placement(written),
                        sectionLocalId = if (written % 50 == 0) sectionId else null,
                        priority = PRIORITIES[written % PRIORITIES.size],
                    ),
                )
                written++
            }

            // Tagging, on most of the workspace: the label lists and the chips on
            // every row are read from these rows.
            // A task filed in a project carries that project's context as well,
            // exactly as it does on the server — which is what lets one query
            // answer for a whole branch, and what makes the context list the one
            // list that legitimately holds a large share of the workspace.
            db.openHelper.writableDatabase.execSQL(
                "UPDATE tasks SET contextLocalId = " +
                    "(SELECT p.contextLocalId FROM projects p WHERE p.localId = tasks.projectLocalId) " +
                    "WHERE projectLocalId IS NOT NULL",
            )

            // A fresh replica hands out device ids from one upwards, so the rows
            // just written are exactly this range.
            // Seven shares no factor with the number of labels, so leaving every
            // seventh task untagged still reaches all of them — a label with no
            // tasks would make its list a measurement of nothing.
            for (taskLocalId in (1L..written.toLong()).filter { it % 7 != 0L }) {
                val labelLocalId = labelIds[(taskLocalId % LABELS).toInt()]
                db.tasks().insertLabels(listOf(TaskLabelRow(taskLocalId, labelLocalId, NOW)))
                taggings++
            }

            repeat(RELATIONS) { index ->
                db.taskRelations().insert(
                    TaskRelationRow(
                        sourceTaskLocalId = (index * 2 + 1).toLong(),
                        targetTaskLocalId = (index * 2 + 2).toLong(),
                        type = if (index % 2 == 0) RelationType.BLOCKS else RelationType.RELATED,
                        createdAt = NOW,
                    ),
                )
            }
        }

        return SeededWorkspace(
            projectLocalIds = projectIds,
            sectionLocalId = sectionId,
            contextLocalId = contextIds.first(),
            labelLocalId = labelIds.first(),
            deepestParentLocalId = deepestParent,
            wideParentLocalId = wideParent,
            subtasks = subtasks,
            taggings = taggings,
        )
    }

    @Suppress("LongParameterList")
    private fun seedTask(
        title: String,
        description: String = "",
        inboxId: Long? = null,
        contextLocalId: Long? = null,
        projectLocalId: Long? = null,
        sectionLocalId: Long? = null,
        parentLocalId: Long? = null,
        priority: Priority = Priority.NONE,
        status: TaskStatus = TaskStatus.OPEN,
        dueAt: Long? = null,
        dayPart: DayPart = DayPart.NONE,
        planState: PlanState = PlanState.NONE,
        pinned: Boolean = false,
        completedAt: Long? = null,
        troikiCategory: TroikiCategory? = null,
    ) = TaskRow(
        title = title,
        description = description,
        inboxId = inboxId,
        contextLocalId = contextLocalId,
        projectLocalId = projectLocalId,
        sectionLocalId = sectionLocalId,
        parentLocalId = parentLocalId,
        priority = priority,
        status = status,
        dueAt = dueAt,
        dayPart = dayPart,
        planState = planState,
        isPinned = pinned,
        pinnedAt = if (pinned) NOW else null,
        completedAt = completedAt,
        troikiCategory = troikiCategory,
        createdAt = NOW,
        updatedAt = NOW + title.hashCode().toLong(),
    )
}

/** The three standing queries a rendered list is joined with, as the app runs them. */
private const val HYDRATION_LABELS =
    "SELECT tl.taskLocalId AS taskLocalId, l.* FROM task_labels tl " +
        "JOIN labels l ON l.localId = tl.labelLocalId ORDER BY l.name COLLATE NOCASE"

private const val HYDRATION_PARENTS =
    "SELECT localId AS taskLocalId, parentLocalId AS parentLocalId FROM tasks WHERE parentLocalId IS NOT NULL"

private const val HYDRATION_EDGES =
    "SELECT r.sourceTaskLocalId AS sourceTaskLocalId, r.targetTaskLocalId AS targetTaskLocalId, " +
        "r.type AS type, (s.status = 'open') AS sourceOpen " +
        "FROM task_relations r JOIN tasks s ON s.localId = r.sourceTaskLocalId"
