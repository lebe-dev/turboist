package ru.tinyops.turboist.nativeapp.labels

import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import ru.tinyops.turboist.core.model.TaskStatus
import ru.tinyops.turboist.core.model.view.LabelUsage
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.core.model.view.labelUsage
import ru.tinyops.turboist.nativeapp.projects.CountingScheduler
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * What the label report does with the workspace it is given.
 *
 * Every check here runs without Compose, without a database and without a
 * server. The counting itself is settled where the rules live; what is asked
 * here is what the screen makes of them — which window is on show, what leads
 * the ranking, what is offered for cleanup, and that switching window never asks
 * anything of the replica.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class LabelStatsPresenterTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")
    private val now: Instant = Instant.parse("2024-03-09T09:34:56.789Z")
    private val today = MutableStateFlow(now)
    private val usage = MutableStateFlow<List<LabelUsage>>(emptyList())
    private val actions = RecordingLabelActions()
    private val sync = CountingScheduler()

    private fun daysAgo(days: Long): Long = now.minus(Duration.ofDays(days)).toEpochMilli()

    private fun TestScope.presenter(): LabelStatsPresenter {
        val presenter = LabelStatsPresenter(backgroundScope, usage, today, zone, actions, sync)
        backgroundScope.launch { presenter.state.collect {} }
        return presenter
    }

    private fun report(
        labels: List<ru.tinyops.turboist.core.model.Label>,
        taggings: List<ru.tinyops.turboist.core.model.view.LabelTagging>,
    ) = labelUsage(labels, taggings, now, zone)

    @Test
    fun `a workspace not read yet is not the same as one with no labels`() =
        runTest {
            val presenter = presenter()

            assertTrue(presenter.state.value.loading)
            assertFalse(presenter.state.value.isEmpty, "nothing has been read, so nothing is known to be empty")
        }

    @Test
    fun `the busiest label leads, and the unused ones are offered for cleanup`() =
        runTest {
            val presenter = presenter()
            usage.value =
                report(
                    listOf(label(1, "hot"), label(2, "warm"), label(3, "cold")),
                    listOf(
                        tagging(1, taggedAt = daysAgo(1)),
                        tagging(1, taggedAt = daysAgo(2)),
                        tagging(2, taggedAt = daysAgo(1)),
                        tagging(3, taggedAt = daysAgo(60)),
                    ),
                )
            runCurrent()

            val state = presenter.state.value
            assertEquals(listOf("hot", "warm"), state.active.map { it.usage.label.name })
            assertEquals(listOf("cold"), state.idle.map { it.usage.label.name })
            assertEquals(3, state.labelCount)
            assertEquals(2, state.mostApplied, "the bars are drawn against the busiest label")
        }

    @Test
    fun `switching the window re-reads numbers already in hand`() =
        runTest {
            val presenter = presenter()
            usage.value =
                report(
                    listOf(label(1, "quarterly")),
                    listOf(tagging(1, taggedAt = daysAgo(60))),
                )
            runCurrent()

            assertEquals(emptyList(), presenter.state.value.active.map { it.usage.label.name })

            presenter.choosePeriod(LabelUsagePeriod.QUARTER)
            runCurrent()

            assertEquals(LabelUsagePeriod.QUARTER, presenter.state.value.period)
            assertEquals(listOf("quarterly"), presenter.state.value.active.map { it.usage.label.name })
            assertEquals(0, sync.requests, "the numbers were already counted; nothing was asked of the replica")
        }

    @Test
    fun `the headline counters follow the window, except the overdue backlog`() =
        runTest {
            val presenter = presenter()
            val longOverdue = now.minus(Duration.ofDays(200)).toEpochMilli()
            usage.value =
                report(
                    listOf(label(1, "one"), label(2, "two")),
                    listOf(
                        tagging(1, taggedAt = daysAgo(1), dueAt = longOverdue),
                        tagging(
                            2,
                            taggedAt = daysAgo(40),
                            status = TaskStatus.COMPLETED,
                            completedAt = daysAgo(1),
                        ),
                    ),
                )
            runCurrent()

            val week = presenter.state.value.totals
            assertEquals(1, week.applied)
            assertEquals(1, week.completed)
            assertEquals(1, week.overdue)

            presenter.choosePeriod(LabelUsagePeriod.QUARTER)
            runCurrent()

            val quarter = presenter.state.value.totals
            assertEquals(2, quarter.applied, "the older application is inside the wider window")
            assertEquals(
                week.overdue,
                quarter.overdue,
                "the overdue backlog is what is carried now, not what happened in a window",
            )
        }

    @Test
    fun `how long ago a label was last reached for is counted in whole days`() =
        runTest {
            val presenter = presenter()
            usage.value =
                report(
                    listOf(label(1, "never"), label(2, "recently")),
                    listOf(tagging(2, taggedAt = daysAgo(30))),
                )
            runCurrent()

            val ages = presenter.state.value.idle.associate { it.usage.label.name to it.lastUsedDaysAgo }
            assertEquals(null, ages.getValue("never"))
            assertEquals(30L, ages.getValue("recently"))
        }

    @Test
    fun `naming a label writes it down`() =
        runTest {
            val presenter = presenter()

            presenter.createLabel("  urgent  ", "red")
            runCurrent()

            assertEquals(listOf("create(urgent,red,false)"), actions.calls)
        }

    @Test
    fun `a label with no name is not written down`() =
        runTest {
            val presenter = presenter()

            presenter.createLabel("   ", "red")
            runCurrent()

            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `a refused write is reported rather than thrown past the screen`() =
        runTest {
            val refusing = RecordingLabelActions(refuse = true)
            val presenter = LabelStatsPresenter(backgroundScope, usage, today, zone, refusing, sync)
            val said = mutableListOf<LabelMessage>()
            backgroundScope.launch { presenter.messages.collect { said += it } }
            runCurrent()

            presenter.createLabel("urgent", "")
            runCurrent()

            assertEquals(listOf(LabelMessage.FAILED), said)
        }

    @Test
    fun `pulling the screen down asks the sync engine to catch up`() =
        runTest {
            val presenter = presenter()

            presenter.refresh()
            runCurrent()

            assertEquals(1, sync.requests)
            assertFalse(presenter.state.value.refreshing, "the gesture is over once the catch-up returns")
        }
}
