package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flow
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.database.dao.OpenBlocker
import ru.tinyops.turboist.core.database.dao.TaskHydrationDao
import ru.tinyops.turboist.core.database.dao.TaskLabelJoin
import ru.tinyops.turboist.core.database.dao.TaskParentLink
import ru.tinyops.turboist.core.database.dao.TaskRelationEdge
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.RelationType
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskRelationSummary
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * What a list pays for the facts joined onto its rows.
 *
 * These pin a cost rather than a result, which is why they count: a screen shows
 * the same thing whether a write is joined once or four times, and the only
 * symptom of losing any of this is a phone that runs warm while it syncs.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class WorkspaceTaskFactsTest {
    private class CountingHydrationDao : TaskHydrationDao {
        val labels = MutableStateFlow<List<TaskLabelJoin>>(emptyList())
        val edges = MutableStateFlow<List<TaskRelationEdge>>(emptyList())
        val parents = MutableStateFlow<List<TaskParentLink>>(emptyList())

        var labelQueries = 0
        var edgeQueries = 0
        var parentQueries = 0

        override fun observeTaskLabels(): Flow<List<TaskLabelJoin>> =
            flow {
                labelQueries++
                labels.collect { emit(it) }
            }

        override fun observeRelationEdges(): Flow<List<TaskRelationEdge>> =
            flow {
                edgeQueries++
                edges.collect { emit(it) }
            }

        override fun observeParentLinks(): Flow<List<TaskParentLink>> =
            flow {
                parentQueries++
                parents.collect { emit(it) }
            }

        override fun observeOpenBlockers(): Flow<List<OpenBlocker>> = flowOf(emptyList())
    }

    private fun TestScope.facts(dao: TaskHydrationDao): WorkspaceTaskFacts = WorkspaceTaskFacts(dao, backgroundScope)

    private fun row(
        localId: Long,
        title: String = "task $localId",
    ): TaskRow = TaskRow(localId = localId, title = title, createdAt = 0, updatedAt = 0)

    private fun labelRow(
        localId: Long,
        name: String,
    ): LabelRow = LabelRow(localId = localId, name = name, createdAt = 0, updatedAt = 0)

    private fun blocks(
        source: Long,
        target: Long,
        sourceOpen: Boolean = true,
    ): TaskRelationEdge = TaskRelationEdge(source, target, RelationType.BLOCKS, sourceOpen)

    @Test
    fun `any number of lists read the workspace once`() =
        runTest {
            val dao = CountingHydrationDao()
            val facts = facts(dao)
            val dispatcher = StandardTestDispatcher(testScheduler)

            repeat(3) {
                backgroundScope.launch {
                    hydratedTasks(flowOf(listOf(row(1))), facts.current, dispatcher).collect {}
                }
            }
            runCurrent()

            assertEquals(1, dao.labelQueries)
            assertEquals(1, dao.edgeQueries)
            assertEquals(1, dao.parentQueries)
        }

    @Test
    fun `a list gets the labels and the relation summary its row needs`() =
        runTest {
            val dao = CountingHydrationDao()
            dao.labels.value = listOf(TaskLabelJoin(2, labelRow(9, "home")))
            dao.parents.value = listOf(TaskParentLink(taskLocalId = 2, parentLocalId = 1))
            dao.edges.value = listOf(blocks(source = 3, target = 1))
            val dispatcher = StandardTestDispatcher(testScheduler)

            val tasks = hydratedTasks(flowOf(listOf(row(2))), facts(dao).current, dispatcher).first()

            val task = tasks.single()
            assertEquals(listOf("home"), task.labels.map { it.name })
            // The blocker sits on the parent, so the subtask inherits it but has no link of its own.
            assertEquals(TaskRelationSummary(blockedByOpen = 1, total = 0), task.relationSummary)
        }

    @Test
    fun `a write that changes nothing about a list does not reach it again`() =
        runTest {
            val dao = CountingHydrationDao()
            val rows = MutableStateFlow(listOf(row(1)))
            val dispatcher = StandardTestDispatcher(testScheduler)
            val seen = mutableListOf<List<Task>>()
            backgroundScope.launch {
                hydratedTasks(rows, facts(dao).current, dispatcher).collect { seen += it }
            }
            runCurrent()
            assertEquals(1, seen.size)

            // A label on a task this list does not hold.
            dao.labels.value = listOf(TaskLabelJoin(99, labelRow(9, "elsewhere")))
            runCurrent()
            // A subtask this list does not hold.
            dao.parents.value = listOf(TaskParentLink(taskLocalId = 98, parentLocalId = 99))
            runCurrent()

            assertEquals(1, seen.size)
        }

    @Test
    fun `a change to a row on the list does reach it`() =
        runTest {
            val dao = CountingHydrationDao()
            val rows = MutableStateFlow(listOf(row(1)))
            val dispatcher = StandardTestDispatcher(testScheduler)
            val seen = mutableListOf<List<Task>>()
            backgroundScope.launch {
                hydratedTasks(rows, facts(dao).current, dispatcher).collect { seen += it }
            }
            runCurrent()

            rows.value = listOf(row(1, title = "renamed"))
            runCurrent()
            dao.labels.value = listOf(TaskLabelJoin(1, labelRow(9, "home")))
            runCurrent()

            assertEquals(listOf("task 1", "renamed", "renamed"), seen.map { it.single().title })
            assertEquals(listOf("home"), seen.last().single().labels.map { it.name })
        }

    @Test
    fun `an answer identical to the last one is not joined again`() =
        runTest {
            val dao = CountingHydrationDao()
            val dispatcher = StandardTestDispatcher(testScheduler)
            val rows = MutableStateFlow(listOf(row(1)))
            var joins = 0
            backgroundScope.launch {
                joinedLatest(rows, facts(dao).current, dispatcher) { _, _ -> joins++ }.collect {}
            }
            runCurrent()
            assertEquals(1, joins)

            // What a query does when an unrelated write wakes it: the same rows, as a new list.
            rows.value = listOf(row(1))
            dao.edges.value = emptyList()
            dao.parents.value = emptyList()
            runCurrent()

            assertEquals(1, joins)
        }

    @Test
    fun `answers that arrive during a join are folded into the next one`() =
        runTest {
            val dao = CountingHydrationDao()
            val dispatcher = StandardTestDispatcher(testScheduler)
            val rows = MutableStateFlow(listOf(row(1)))
            val joined = mutableListOf<String>()
            val results = mutableListOf<String>()
            backgroundScope.launch {
                joinedLatest(rows, facts(dao).current, dispatcher) { list, _ ->
                    val title = list.single().title
                    joined += title
                    delay(JOIN_MILLIS)
                    title
                }.collect { results += it }
            }
            runCurrent()

            // The burst a page of a sync produces, all of it landing during the first join.
            rows.value = listOf(row(1, title = "second"))
            runCurrent()
            rows.value = listOf(row(1, title = "third"))
            runCurrent()
            rows.value = listOf(row(1, title = "fourth"))
            runCurrent()
            advanceTimeBy(JOIN_MILLIS * 2)
            runCurrent()

            assertEquals(listOf("task 1", "fourth"), joined)
            assertEquals("fourth", results.last())
        }

    @Test
    fun `the workspace stops being read once nothing has watched it for a while`() =
        runTest {
            val dao = CountingHydrationDao()
            val facts = facts(dao)

            val job = backgroundScope.launch { facts.current.collect {} }
            runCurrent()
            job.cancel()
            // Twice: the indexed facts let go of the queries only once they stop themselves.
            advanceTimeBy(WorkspaceTaskFacts.STOP_TIMEOUT_MILLIS * 2 + 1)
            runCurrent()

            // Changed while nobody was looking; a later reader must not be handed the old answer.
            dao.labels.value = listOf(TaskLabelJoin(1, labelRow(9, "fresh")))
            val after = facts.current.first()

            assertEquals(2, dao.labelQueries)
            assertEquals(listOf("fresh"), after.labelsByTask.getValue(1).map { it.name })
        }

    @Test
    fun `moving between screens does not restart the queries`() =
        runTest {
            val dao = CountingHydrationDao()
            val facts = facts(dao)

            val leaving = backgroundScope.launch { facts.current.collect {} }
            runCurrent()
            leaving.cancel()
            advanceTimeBy(WorkspaceTaskFacts.STOP_TIMEOUT_MILLIS / 2)
            backgroundScope.launch { facts.current.collect {} }
            advanceTimeBy(WorkspaceTaskFacts.STOP_TIMEOUT_MILLIS * 2)
            runCurrent()

            assertEquals(1, dao.labelQueries)
        }

    private companion object {
        const val JOIN_MILLIS: Long = 100
    }
}
