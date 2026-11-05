package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.conflate
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flowOn
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.shareIn
import ru.tinyops.turboist.core.database.dao.TaskHydrationDao
import ru.tinyops.turboist.core.database.dao.TaskLabelJoin
import ru.tinyops.turboist.core.database.dao.TaskParentLink
import ru.tinyops.turboist.core.database.dao.TaskRelationEdge
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import ru.tinyops.turboist.core.model.view.BlockEdge
import ru.tinyops.turboist.core.model.view.taskRelationSummaries
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import javax.inject.Inject
import javax.inject.Singleton

/** What a stored task row does not carry, indexed for joining onto rows. */
data class TaskFacts(
    val labelsByTask: Map<Long, List<Label>>,
    val openBlockEdges: List<BlockEdge>,
    val relationCounts: Map<Long, Int>,
    val parentOf: Map<Long, Long>,
)

/**
 * The workspace-wide facts every list joins on, read once however many lists are
 * open.
 *
 * Each of these queries answers for the whole workspace and is woken by every
 * write the replica takes, a page of a sync included. Asked per list, the today
 * screen alone (today plus overdue) would run six of them on each commit and
 * index the answers twice. Sharing them leaves three, and [distinctUntilChanged]
 * means an answer identical to the last one — what a write to an unrelated column
 * produces — goes no further than the comparison.
 *
 * The queries outlive a brief gap in readers, so moving between screens does not
 * restart them, and stop soon after the last reader goes, so an app put away
 * reads nothing. Nothing is replayed once they stop: the rows may have changed by
 * the time a reader returns, and the first thing it must see is the current
 * answer rather than the last one anybody saw.
 */
@Singleton
class WorkspaceTaskFacts
    @Inject
    constructor(
        hydration: TaskHydrationDao,
        @param:ApplicationScope private val scope: CoroutineScope,
    ) {
        val taskLabels: Flow<List<TaskLabelJoin>> = hydration.observeTaskLabels().sharedWhileWatched()

        val relationEdges: Flow<List<TaskRelationEdge>> = hydration.observeRelationEdges().sharedWhileWatched()

        val parentLinks: Flow<List<TaskParentLink>> = hydration.observeParentLinks().sharedWhileWatched()

        /** The three together, indexed once for every list reading them. */
        val current: Flow<TaskFacts> =
            combine(taskLabels, relationEdges, parentLinks, ::indexed).sharedWhileWatched()

        private fun <T> Flow<T>.sharedWhileWatched(): Flow<T> =
            distinctUntilChanged().shareIn(
                scope,
                SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS, replayExpirationMillis = 0),
                replay = 1,
            )

        companion object {
            /** Long enough to span a screen change, short enough to stop soon after the app is put away. */
            const val STOP_TIMEOUT_MILLIS: Long = 5_000
        }
    }

private fun indexed(
    labels: List<TaskLabelJoin>,
    edges: List<TaskRelationEdge>,
    parents: List<TaskParentLink>,
): TaskFacts =
    TaskFacts(
        labelsByTask = labels.groupBy({ it.taskLocalId }, { it.label.toDomain() }),
        openBlockEdges = openBlockEdgesOf(edges),
        relationCounts = relationCountsOf(edges),
        parentOf = parents.associate { it.taskLocalId to it.parentLocalId },
    )

/** A list's rows with their labels and relation rollup joined on. */
internal fun hydratedTasks(
    rows: Flow<List<TaskRow>>,
    facts: Flow<TaskFacts>,
    dispatcher: CoroutineDispatcher = Dispatchers.Default,
): Flow<List<Task>> = joinedLatest(rows, facts, dispatcher, ::hydrate).distinctUntilChanged()

/**
 * Runs [join] over the latest rows and facts, as seldom as staying current allows.
 *
 * Rows identical to the last answer are dropped before the join, and anything
 * that arrives while a join is running replaces whatever was waiting, so a burst
 * of commits costs one join for the first and one for the last rather than one
 * each. Nothing is delayed to achieve that: a lone change, such as the tick a
 * user is waiting to see land, is joined the moment it arrives.
 *
 * It runs away from the main thread because the rows of a long list are real
 * work to map, and the screens are standing queries, so there is no moment when
 * doing it on the thread that draws would not cost a frame.
 */
internal fun <T> joinedLatest(
    rows: Flow<List<TaskRow>>,
    facts: Flow<TaskFacts>,
    dispatcher: CoroutineDispatcher,
    join: suspend (List<TaskRow>, TaskFacts) -> T,
): Flow<T> =
    combine(rows.distinctUntilChanged(), facts, ::Pair)
        .conflate()
        .map { (latestRows, latestFacts) -> join(latestRows, latestFacts) }
        .flowOn(dispatcher)

private fun hydrate(
    rows: List<TaskRow>,
    facts: TaskFacts,
): List<Task> {
    if (rows.isEmpty()) return emptyList()
    val summaries =
        taskRelationSummaries(
            taskLocalIds = rows.map { it.localId },
            openBlockEdges = facts.openBlockEdges,
            relationCounts = facts.relationCounts,
            parentOf = facts.parentOf,
        )
    return rows.map { row ->
        row.toDomain().copy(
            labels = facts.labelsByTask[row.localId].orEmpty(),
            relationSummary = summaries[row.localId] ?: TaskRelationSummary.NONE,
        )
    }
}
