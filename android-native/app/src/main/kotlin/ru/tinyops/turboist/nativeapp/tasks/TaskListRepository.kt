package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.ProjectSectionDao
import ru.tinyops.turboist.core.database.dao.SettingsDao
import ru.tinyops.turboist.core.database.dao.TaskHydrationDao
import ru.tinyops.turboist.core.database.dao.TaskLabelJoin
import ru.tinyops.turboist.core.database.dao.TaskParentLink
import ru.tinyops.turboist.core.database.dao.TaskRelationEdge
import ru.tinyops.turboist.core.database.dao.TaskViewDao
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.PlanState
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.model.view.taskRelationSummaries
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.UserSettingsDto
import ru.tinyops.turboist.core.network.mapping.toUserSettings
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the task lists read.
 *
 * Every list is a standing query against the replica, so a screen has no
 * "loading from the network" state and no refetch of its own: a change made on
 * this device and a change that arrived from another one reach the screen by the
 * same route, the moment the rows behind it change.
 *
 * A stored task row carries only its own columns, so this is also where the
 * three things a rendered row needs beside them are joined on: the labels it
 * carries, how many relations it has, and whether anything still open stands in
 * its way. They are folded in here rather than in a screen so every list agrees
 * about them.
 */
@Singleton
class TaskListRepository
    @Inject
    constructor(
        private val views: TaskViewDao,
        private val hydration: TaskHydrationDao,
        private val projects: ProjectDao,
        private val sections: ProjectSectionDao,
        private val settings: SettingsDao,
    ) {
        /**
         * Open tasks sitting in the inbox.
         *
         * Every row here is a root: a subtask cannot be filed in the inbox, so
         * the list is flat by construction rather than by being flattened.
         */
        fun observeInbox(): Flow<List<Task>> = hydrated(views.queryInbox(null))

        /** Open tasks due on the day beginning at [dayStart]. */
        fun observeToday(dayStart: Long): Flow<List<Task>> = hydrated(views.queryToday(dayStart))

        /** Open tasks due on the day after the one beginning at [dayStart]. */
        fun observeTomorrow(dayStart: Long): Flow<List<Task>> = hydrated(views.queryTomorrow(dayStart))

        /** Open tasks whose due date has already gone by. */
        fun observeOverdue(dayStart: Long): Flow<List<Task>> = hydrated(views.queryOverdue(dayStart, null))

        /**
         * The week: what the user planned for it, what falls due inside it, and
         * the open work beneath either.
         */
        fun observeWeek(window: TimeWindow): Flow<List<Task>> =
            hydrated(views.queryWeek(window.from, window.untilExclusive, null))

        /**
         * Only the tasks committed to the week.
         *
         * The planning screen is about decisions, so a task that is merely due
         * inside the window does not belong on it — it was never chosen, and
         * offering to park it would suggest it had been.
         */
        fun observePlannedForWeek(window: TimeWindow): Flow<List<Task>> =
            observeWeek(window).map { tasks -> tasks.filter { it.planState == PlanState.WEEK } }

        /** Open tasks parked out of the current week. */
        fun observeBacklog(): Flow<List<Task>> = hydrated(views.queryBacklog(null))

        /**
         * The finished work the replica holds inside [window], most recently
         * completed first.
         *
         * Paged where no other list is, because the history is the one list with
         * no natural end: every other one is bounded by the work that is still
         * open. [limit] is how much of it is being drawn, not how much there is —
         * [observeCompletedCount] answers that.
         */
        fun observeCompleted(
            window: TimeWindow,
            limit: Int,
        ): Flow<List<Task>> = hydrated(views.queryCompletedBetween(window.from, window.untilExclusive, limit, 0, null))

        /** How much finished work the replica holds inside [window], drawn or not. */
        fun observeCompletedCount(window: TimeWindow): Flow<Int> =
            views.observeCompletedCount(window.from, window.untilExclusive)

        /**
         * Every task of one project, whatever its status.
         *
         * A project page is the project's whole history, so finished work stays
         * on it: ticking a task off on a board moves it into the finished group
         * of its column rather than making it vanish.
         */
        fun observeProject(projectLocalId: Long): Flow<List<Task>> = hydrated(views.queryProject(projectLocalId, null))

        /**
         * Every task carrying one label, whatever its status.
         *
         * A label is a cross-cutting view of the workspace rather than a place
         * work lives, so finished and abandoned tasks stay on it: the question
         * a label answers is "what did I mark this way", not "what is left to
         * do".
         */
        fun observeLabel(labelLocalId: Long): Flow<List<Task>> = hydrated(views.queryLabel(labelLocalId, null))

        /**
         * Every task under a context, the tasks of its projects included.
         *
         * A task filed in a project carries its context too, which is what lets
         * one query answer for the whole branch rather than one per project.
         */
        fun observeContext(contextLocalId: Long): Flow<List<Task>> = hydrated(views.queryContext(contextLocalId, null))

        /**
         * Every task of the projects standing in the daily plan, in one query.
         *
         * One query for all of them rather than one per project: the plan draws
         * up to nine projects side by side, and nine standing queries over the
         * same table would each be woken by every write the replica takes.
         *
         * Abandoned work and everything beneath it is already left out by the
         * query, and finished work is kept, so a task ticked off on the plan
         * moves into the finished part of its project instead of vanishing.
         *
         * An empty set of projects is answered without asking the database: a
         * plan nobody has assigned anything to yet has no work in it, and the
         * question "which tasks are in none of these projects" has no form SQL
         * accepts.
         */
        fun observeTroikiBoard(projectLocalIds: Collection<Long>): Flow<List<Task>> {
            if (projectLocalIds.isEmpty()) return flowOf(emptyList())
            return hydrated(views.queryTroikiBoard(projectLocalIds))
        }

        /**
         * Where a selection can be sent: every project, with its columns under it.
         *
         * Read as one standing query like everything else a list shows, so a
         * project created on another device turns up in the picker as soon as it
         * is replicated rather than when the screen is next opened.
         */
        fun observeMoveOptions(): Flow<List<MoveProject>> =
            combine(projects.observeAll(), sections.observeAll()) { projectRows, sectionRows ->
                val columns = sectionRows.groupBy { it.projectLocalId }
                projectRows.map { project ->
                    MoveProject(
                        projectLocalId = project.localId,
                        title = project.title,
                        sections =
                            columns[project.localId].orEmpty().map {
                                MoveSection(it.localId, it.title)
                            },
                    )
                }
            }

        /** Project titles by local id, for the folder a row names. */
        fun observeProjectTitles(): Flow<Map<Long, String>> =
            projects.observeAll().map { rows -> rows.associate { it.localId to it.title } }

        /**
         * The user's own preferences as last replicated.
         *
         * A blob this build cannot read falls back to the defaults rather than
         * failing: a preference document written by a newer server must not be
         * able to blank a screen.
         */
        fun observeUserSettings(): Flow<UserSettings> =
            settings.observeUserSettings().map { row ->
                val payload = row?.payload ?: return@map UserSettings()
                runCatching { TurboistJson.decodeFromString(UserSettingsDto.serializer(), payload) }
                    .map { it.toUserSettings() }
                    .getOrElse { UserSettings() }
            }

        /**
         * Joins the labels, the relation rollup and nothing else onto a list's rows.
         *
         * The three standing queries are combined with the list rather than
         * rebuilt for the rows it happens to contain, so tagging a task or
         * finishing a blocker redraws the list without the queries behind it
         * being torn down and set up again.
         *
         * The join runs away from the main thread, and that is a requirement
         * rather than tidiness. Each of the three queries answers for the whole
         * workspace, so on a well-used one the join indexes several thousand
         * taggings and edges — and it runs again on every write the replica
         * takes, including each page a sync applies. Left on the thread that
         * draws, that is a dropped frame every time anything changes; the screens
         * are standing queries, so there is no moment when it would not be.
         */
        private fun hydrated(rows: Flow<List<TaskRow>>): Flow<List<Task>> =
            combine(
                rows,
                hydration.observeTaskLabels(),
                hydration.observeRelationEdges(),
                hydration.observeParentLinks(),
            ) { tasks, labels, edges, parents -> hydrate(tasks, labels, edges, parents) }
                .flowOn(Dispatchers.Default)

        private fun hydrate(
            rows: List<TaskRow>,
            labels: List<TaskLabelJoin>,
            edges: List<TaskRelationEdge>,
            parents: List<TaskParentLink>,
        ): List<Task> {
            if (rows.isEmpty()) return emptyList()
            val labelsByTask = labels.groupBy({ it.taskLocalId }, { it.label.toDomain() })
            val summaries =
                taskRelationSummaries(
                    taskLocalIds = rows.map { it.localId },
                    openBlockEdges = openBlockEdgesOf(edges),
                    relationCounts = relationCountsOf(edges),
                    parentOf = parents.associate { it.taskLocalId to it.parentLocalId },
                )
            return rows.map { row ->
                row.toDomain().copy(
                    labels = labelsByTask[row.localId].orEmpty(),
                    relationSummary = summaries[row.localId] ?: TaskRelationSummary.NONE,
                )
            }
        }
    }
