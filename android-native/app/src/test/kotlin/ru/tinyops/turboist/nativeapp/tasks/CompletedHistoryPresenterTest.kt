package ru.tinyops.turboist.nativeapp.tasks

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.NO_LOCAL_ID
import ru.tinyops.turboist.core.model.Task
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The completion history at its edge.
 *
 * The screen reads two sources and has to be honest about which one it is at: it
 * shows more of the device's own copy for as long as there is more, goes to the
 * server when there is not, and — when it cannot reach the server — says so
 * instead of spinning. All of that is settled here, with no database, no server
 * and no Compose: it is a question about a state machine, not about drawing.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class CompletedHistoryPresenterTest {
    private val zone: ZoneId = ZoneId.of("UTC")
    private val today = MutableStateFlow(LocalDate.of(2026, 3, 1))

    /** The first instant the device still holds finished work from. */
    private val boundary = Instant.parse("2025-12-01T00:00:00Z").toEpochMilli()

    private val history = FakeHistory(boundary)
    private val older = ScriptedOlderCompletions()
    private val actions = RecordingActions()
    private val sync = CountingScheduler()

    @Test
    fun `the device's own history is grouped by the day it was finished`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"), finished(2, "2026-02-28T09:00:00Z"))
            val presenter = watch()

            assertEquals(
                listOf(LocalDate.of(2026, 3, 1), LocalDate.of(2026, 2, 28)),
                presenter.state.value.sections.map { it.day },
            )
        }

    @Test
    fun `while the device holds more than is drawn the screen shows its own before asking the server`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"))
            history.count.value = 400
            val presenter = watch()

            assertEquals(OlderHistory.IN_REPLICA, presenter.state.value.older)

            presenter.showMore()
            runCurrent()

            assertEquals(0, older.requests.size, "the server is not asked while the device still has lines")
        }

    @Test
    fun `the first request to the server starts where the device's own copy ends`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"))
            history.count.value = 1
            older.answer(OlderCompletionsResult.Page(tasks = emptyList(), nextOffset = 1, hasMore = false))
            val presenter = watch()

            assertEquals(OlderHistory.ON_SERVER, presenter.state.value.older)
            presenter.showMore()
            runCurrent()

            assertEquals(listOf(1), older.requests.map { it.first })
        }

    @Test
    fun `a fetched page is shown below the device's own history and marked as fetched`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"))
            history.count.value = 1
            older.answer(
                OlderCompletionsResult.Page(
                    tasks = listOf(fetched(90, "2025-06-05T09:00:00Z")),
                    nextOffset = 2,
                    hasMore = true,
                ),
            )
            val presenter = watch()

            presenter.showMore()
            runCurrent()

            val sections = presenter.state.value.sections
            assertEquals(listOf(LocalDate.of(2026, 3, 1), LocalDate.of(2025, 6, 5)), sections.map { it.day })
            assertEquals(CompletedSource.SERVER, sections.last().rows.single().source)
            assertEquals(OlderHistory.ON_SERVER, presenter.state.value.older)
        }

    @Test
    fun `with no connection the boundary says so and nothing is left pending`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"))
            history.count.value = 1
            older.answer(OlderCompletionsResult.Offline)
            val presenter = watch()

            presenter.showMore()
            runCurrent()

            assertEquals(OlderHistory.OFFLINE, presenter.state.value.older)
            assertTrue(presenter.state.value.sections.isNotEmpty(), "what the device holds is still on screen")
        }

    @Test
    fun `a refusal by the server is told apart from having no connection`() =
        runTest {
            history.count.value = 0
            older.answer(OlderCompletionsResult.Failed)
            val presenter = watch()

            presenter.showMore()
            runCurrent()

            assertEquals(OlderHistory.FAILED, presenter.state.value.older)
        }

    @Test
    fun `a server that has nothing older is reported as the end of the history`() =
        runTest {
            history.count.value = 0
            older.answer(OlderCompletionsResult.Page(tasks = emptyList(), nextOffset = 0, hasMore = false))
            val presenter = watch()

            presenter.showMore()
            runCurrent()

            assertEquals(OlderHistory.EXHAUSTED, presenter.state.value.older)
        }

    @Test
    fun `pages that only repeat what the device holds are stepped over, then the offer stands`() =
        runTest {
            history.count.value = 0
            repeat(6) {
                older.answer(
                    OlderCompletionsResult.Page(
                        // Inside the window, so the device already has it.
                        tasks = listOf(fetched(90L + it, "2026-01-05T09:00:00Z")),
                        nextOffset = it + 1,
                        hasMore = true,
                    ),
                )
            }
            val presenter = watch()

            presenter.showMore()
            runCurrent()

            assertTrue(older.requests.size in 2..4, "a drifted boundary is stepped over, a whole history is not")
            assertEquals(OlderHistory.ON_SERVER, presenter.state.value.older)
            assertTrue(presenter.state.value.sections.isEmpty(), "nothing older than the window came back")
        }

    @Test
    fun `reopening a line the device holds names it by the device's own id`() =
        runTest {
            history.holds(finished(1, "2026-03-01T09:00:00Z"))
            val presenter = watch()

            presenter.uncomplete(presenter.state.value.sections.first().rows.single())
            runCurrent()

            assertEquals(listOf(1L), actions.reopened.map { it.localId })
        }

    @Test
    fun `reopening a fetched line names it the way the server does and takes it off the screen`() =
        runTest {
            history.count.value = 0
            older.answer(
                OlderCompletionsResult.Page(
                    tasks = listOf(fetched(90, "2025-06-05T09:00:00Z")),
                    nextOffset = 1,
                    hasMore = false,
                ),
            )
            val presenter = watch()
            presenter.showMore()
            runCurrent()

            presenter.uncomplete(presenter.state.value.sections.single().rows.single())
            runCurrent()

            assertEquals(listOf(90L), actions.reopened.map { it.serverId })
            assertEquals(NO_LOCAL_ID, actions.reopened.single().localId)
            assertTrue(presenter.state.value.sections.isEmpty(), "it is no longer finished, so it leaves the history")
        }

    @Test
    fun `a catch-up forgets the fetched pages`() =
        runTest {
            history.count.value = 0
            older.answer(
                OlderCompletionsResult.Page(
                    tasks = listOf(fetched(90, "2025-06-05T09:00:00Z")),
                    nextOffset = 1,
                    hasMore = false,
                ),
            )
            val presenter = watch()
            presenter.showMore()
            runCurrent()
            assertTrue(presenter.state.value.sections.isNotEmpty())

            presenter.refresh()
            runCurrent()

            assertEquals(1, sync.requests)
            assertTrue(presenter.state.value.sections.isEmpty(), "the fetched copy went with the gesture")
        }

    /** Starts the presenter with something collecting it, which is what a screen does. */
    private fun TestScope.watch(): CompletedHistoryPresenter {
        val presenter =
            CompletedHistoryPresenter(
                scope = backgroundScope,
                history = history,
                older = older,
                actions = actions,
                sync = sync,
                zone = zone,
                today = today,
            )
        backgroundScope.launch { presenter.state.collect {} }
        runCurrent()
        return presenter
    }

    private fun finished(
        localId: Long,
        at: String,
    ): Task =
        task(
            localId = localId,
            serverId = localId,
            status = TaskStatus.COMPLETED,
            completedAt = Instant.parse(at).toEpochMilli(),
        )

    private fun fetched(
        serverId: Long,
        at: String,
    ): Task =
        task(
            localId = NO_LOCAL_ID,
            serverId = serverId,
            status = TaskStatus.COMPLETED,
            completedAt = Instant.parse(at).toEpochMilli(),
        )

    /** The device's own copy of the history, stated rather than queried. */
    private class FakeHistory(
        private val boundary: Long,
    ) : CompletedHistorySource {
        val rows = MutableStateFlow<List<Task>>(emptyList())
        val count = MutableStateFlow(0)
        val titles = MutableStateFlow<Map<Long, String>>(emptyMap())

        fun holds(vararg tasks: Task) {
            rows.value = tasks.toList()
            count.value = tasks.size
        }

        override fun observe(limit: Int): Flow<List<Task>> = rows.map { it.take(limit) }

        override fun observeCount(): Flow<Int> = count

        override fun observeProjectTitles(): Flow<Map<Long, String>> = titles

        override fun windowStart(): Long = boundary
    }

    /** The server's half, answered from a script and remembering what it was asked. */
    private class ScriptedOlderCompletions : OlderCompletions {
        val requests = mutableListOf<Pair<Int, Int>>()
        private val answers = ArrayDeque<OlderCompletionsResult>()

        fun answer(result: OlderCompletionsResult) {
            answers += result
        }

        override suspend fun page(
            offset: Int,
            limit: Int,
        ): OlderCompletionsResult {
            requests += offset to limit
            return answers.removeFirstOrNull()
                ?: OlderCompletionsResult.Page(emptyList(), nextOffset = offset, hasMore = false)
        }
    }

    private class RecordingActions : CompletedHistoryActions {
        val reopened = mutableListOf<Task>()

        override suspend fun uncomplete(task: Task) {
            reopened += task
        }
    }

    private class CountingScheduler : SyncScheduler {
        var requests = 0

        override suspend fun requestSyncNow() {
            requests++
        }
    }
}
