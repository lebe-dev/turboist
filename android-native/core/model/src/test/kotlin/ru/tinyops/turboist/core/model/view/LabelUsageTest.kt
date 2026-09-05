package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.TaskStatus
import java.time.Duration
import java.time.Instant
import java.time.ZoneId
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * The label usage report, which the device computes for itself.
 *
 * The same question the server answers, asked of the replica instead, so the
 * screen works with no connection. The cases below are the server's own, kept
 * side by side with it: what counts as an application, what counts as a
 * completion, which of the two a cancelled task is neither of, and where a
 * rolling window's edges fall.
 *
 * Every case runs against a stopped clock in a zone that is not UTC. The window
 * boundaries are calendar boundaries — midnight where the user is — so a report
 * computed in the wrong zone is silently off by hours, and only a fixed clock in
 * an offset zone can catch that.
 */
class LabelUsageTest {
    private val zone: ZoneId = ZoneId.of("Europe/Moscow")

    /** Midday, so "some hours ago" and "some days ago" never cross a boundary by accident. */
    private val now: Instant = Instant.parse("2024-03-09T09:34:56.789Z")

    private var nextLocalId = 0L

    private fun label(name: String): Label {
        nextLocalId += 1
        return Label(localId = nextLocalId, name = name, createdAt = 0L, updatedAt = 0L)
    }

    private fun daysAgo(days: Long): Long = now.minus(Duration.ofDays(days)).toEpochMilli()

    private fun tagging(
        label: Label,
        taggedAt: Long? = daysAgo(1),
        status: TaskStatus = TaskStatus.OPEN,
        dueAt: Long? = null,
        completedAt: Long? = null,
        projectLocalId: Long? = null,
    ) = LabelTagging(
        labelLocalId = label.localId,
        taggedAt = taggedAt,
        status = status,
        dueAt = dueAt,
        completedAt = completedAt,
        projectLocalId = projectLocalId,
    )

    private fun report(
        labels: List<Label>,
        taggings: List<LabelTagging>,
    ): Map<String, LabelUsage> = labelUsage(labels, taggings, now, zone).associateBy { it.label.name }

    @Test
    fun `an application lands in every window it is old enough for`() {
        val hot = label("hot")
        val cold = label("cold")
        val unused = label("unused")

        val rows =
            report(
                listOf(hot, cold, unused),
                listOf(
                    tagging(hot, taggedAt = daysAgo(1)),
                    tagging(hot, taggedAt = daysAgo(3)),
                    tagging(hot, taggedAt = daysAgo(21)),
                    tagging(cold, taggedAt = daysAgo(45)),
                ),
            )

        val hotRow = rows.getValue("hot")
        assertEquals(2, hotRow.period(LabelUsagePeriod.WEEK).applied)
        assertEquals(3, hotRow.period(LabelUsagePeriod.MONTH).applied)
        // The week's comparison window is days 8..14 back; a 21-day-old tagging is in neither.
        assertEquals(0, hotRow.period(LabelUsagePeriod.WEEK).previousApplied)
        assertEquals(0, hotRow.period(LabelUsagePeriod.MONTH).previousApplied)
        assertEquals(3, hotRow.totalTasks)
        assertEquals(daysAgo(1), hotRow.lastUsedAt)

        val coldRow = rows.getValue("cold")
        assertEquals(0, coldRow.period(LabelUsagePeriod.WEEK).applied)
        assertEquals(0, coldRow.period(LabelUsagePeriod.MONTH).applied)
        // 45 days back is inside the month window's comparison bucket, days 31..60.
        assertEquals(1, coldRow.period(LabelUsagePeriod.MONTH).previousApplied)
        assertEquals(1, coldRow.totalTasks)

        val unusedRow = rows.getValue("unused")
        assertEquals(0, unusedRow.totalTasks, "a label nobody used is still reported, with zeroes")
        assertNull(unusedRow.lastUsedAt)
    }

    @Test
    fun `open, overdue and completed are counted apart`() {
        val work = label("work")
        val todayStart = ViewWindows.dayStart(now, zone)

        val rows =
            report(
                listOf(work),
                listOf(
                    tagging(work, taggedAt = daysAgo(2)),
                    tagging(work, taggedAt = daysAgo(2), dueAt = todayStart - 3 * ViewWindows.DAY_MILLIS),
                    tagging(
                        work,
                        taggedAt = daysAgo(2),
                        status = TaskStatus.COMPLETED,
                        completedAt = daysAgo(1),
                    ),
                ),
            )

        val row = rows.getValue("work")
        assertEquals(2, row.openTasks)
        assertEquals(1, row.overdue)
        assertEquals(1, row.period(LabelUsagePeriod.WEEK).completed)
        assertEquals(3, row.totalTasks)
    }

    @Test
    fun `the spread counts projects, and work with no project is in none`() {
        val spread = label("spread")

        val rows =
            report(
                listOf(spread),
                listOf(
                    tagging(spread, projectLocalId = 7L),
                    tagging(spread, projectLocalId = 7L),
                    tagging(spread, projectLocalId = 9L),
                    tagging(spread, projectLocalId = null),
                ),
            )

        val row = rows.getValue("spread")
        assertEquals(2, row.projects)
        assertEquals(4, row.totalTasks)
    }

    @Test
    fun `a tagging with no moment counts in the totals and in no window`() {
        val legacy = label("legacy")

        val rows = report(listOf(legacy), listOf(tagging(legacy, taggedAt = null)))

        val row = rows.getValue("legacy")
        assertEquals(1, row.totalTasks)
        assertEquals(1, row.openTasks)
        for (period in LabelUsagePeriod.entries) {
            assertEquals(0, row.period(period).applied, "$period applied")
            assertEquals(0, row.period(period).previousApplied, "$period previously applied")
        }
        assertNull(row.lastUsedAt, "a tagging with no moment cannot be the most recent one")
    }

    @Test
    fun `abandoned work is neither open nor finished, but it was still tagged`() {
        val dropped = label("dropped")

        val rows =
            report(
                listOf(dropped),
                listOf(
                    tagging(
                        dropped,
                        taggedAt = daysAgo(2),
                        status = TaskStatus.CANCELLED,
                        // A cancelled row can still carry a stale completion moment; the
                        // status is what settles whether any work was finished.
                        completedAt = daysAgo(1),
                    ),
                ),
            )

        val row = rows.getValue("dropped")
        assertEquals(1, row.totalTasks)
        assertEquals(0, row.openTasks)
        assertEquals(1, row.period(LabelUsagePeriod.WEEK).applied)
        assertEquals(0, row.period(LabelUsagePeriod.WEEK).completed)
    }

    @Test
    fun `a completion is bucketed by when work finished, not by when it was tagged`() {
        val longhaul = label("longhaul")

        val rows =
            report(
                listOf(longhaul),
                listOf(
                    tagging(
                        longhaul,
                        taggedAt = daysAgo(100),
                        status = TaskStatus.COMPLETED,
                        completedAt = daysAgo(1),
                    ),
                ),
            )

        val row = rows.getValue("longhaul")
        for (period in LabelUsagePeriod.entries) {
            assertEquals(1, row.period(period).completed, "$period completed")
            assertEquals(0, row.period(period).applied, "$period applied — tagged 100 days ago")
        }
    }

    @Test
    fun `a completion outside a window does not leak into it`() {
        val stale = label("stale")

        val rows =
            report(
                listOf(stale),
                listOf(
                    tagging(
                        stale,
                        taggedAt = daysAgo(50),
                        status = TaskStatus.COMPLETED,
                        completedAt = daysAgo(40),
                    ),
                ),
            )

        val row = rows.getValue("stale")
        assertEquals(0, row.period(LabelUsagePeriod.WEEK).completed)
        assertEquals(0, row.period(LabelUsagePeriod.MONTH).completed)
        assertEquals(1, row.period(LabelUsagePeriod.QUARTER).completed)
        assertEquals(1, row.period(LabelUsagePeriod.QUARTER).applied)
    }

    @Test
    fun `the last use is the most recent application`() {
        val recent = label("recent")
        val newest = daysAgo(2)

        val rows =
            report(
                listOf(recent),
                listOf(
                    tagging(recent, taggedAt = daysAgo(30)),
                    tagging(recent, taggedAt = newest),
                    tagging(recent, taggedAt = daysAgo(9)),
                ),
            )

        assertEquals(newest, rows.getValue("recent").lastUsedAt)
    }

    @Test
    fun `a workspace with no labels reports nothing`() {
        assertEquals(emptyList(), labelUsage(emptyList(), emptyList(), now, zone))
    }

    // --- where the windows begin and end ---

    @Test
    fun `a window ends at the end of today, so this morning still counts`() {
        val today = label("today")
        val window = labelUsageWindow(now, zone, LabelUsagePeriod.WEEK)
        val endOfToday = window.untilExclusive

        val rows =
            report(
                listOf(today),
                listOf(
                    tagging(today, taggedAt = endOfToday - 1),
                    tagging(today, taggedAt = endOfToday),
                ),
            )

        assertEquals(
            1,
            rows.getValue("today").period(LabelUsagePeriod.WEEK).applied,
            "the last millisecond of today is inside the window and midnight is not",
        )
    }

    @Test
    fun `the first millisecond of a window is inside it and the one before is not`() {
        val edge = label("edge")
        val window = labelUsageWindow(now, zone, LabelUsagePeriod.WEEK)

        val rows =
            report(
                listOf(edge),
                listOf(
                    tagging(edge, taggedAt = window.from),
                    tagging(edge, taggedAt = window.from - 1),
                ),
            )

        val row = rows.getValue("edge")
        assertEquals(1, row.period(LabelUsagePeriod.WEEK).applied)
        assertEquals(
            1,
            row.period(LabelUsagePeriod.WEEK).previousApplied,
            "the millisecond before belongs to the window before",
        )
    }

    @Test
    fun `the windows are measured where the user is, not in UTC`() {
        val utc = labelUsageWindow(now, ZoneId.of("UTC"), LabelUsagePeriod.WEEK)
        val moscow = labelUsageWindow(now, zone, LabelUsagePeriod.WEEK)

        assertEquals(
            Duration.ofHours(-3).toMillis(),
            moscow.untilExclusive - utc.untilExclusive,
            "Moscow's midnight arrives three hours before UTC's",
        )
        assertEquals(
            moscow.untilExclusive - moscow.from,
            utc.untilExclusive - utc.from,
            "both windows are the same length; only where they are cut differs",
        )
    }

    @Test
    fun `the comparison window is the same length and ends where the window begins`() {
        val window = labelUsageWindow(now, zone, LabelUsagePeriod.MONTH)
        val previous = previousWindow(window)

        assertEquals(window.from, previous.untilExclusive)
        assertEquals(window.untilExclusive - window.from, previous.untilExclusive - previous.from)
    }

    @Test
    fun `the quarter covers every window a report offers`() {
        val quarter = labelUsageWindow(now, zone, LabelUsagePeriod.QUARTER)

        for (period in LabelUsagePeriod.entries) {
            val window = labelUsageWindow(now, zone, period)
            assertTrue(window.from >= quarter.from, "$period begins inside the quarter")
            assertEquals(quarter.untilExclusive, window.untilExclusive, "$period ends where the quarter ends")
        }
    }

    // --- how the report is read on screen ---

    @Test
    fun `the ranking puts the most used first and settles ties by name`() {
        val a = label("alpha")
        val b = label("bravo")
        val c = label("charlie")

        val rows =
            labelUsage(
                listOf(c, b, a),
                listOf(
                    tagging(c, taggedAt = daysAgo(1)),
                    tagging(b, taggedAt = daysAgo(1)),
                    tagging(a, taggedAt = daysAgo(1)),
                    tagging(a, taggedAt = daysAgo(2)),
                ),
                now,
                zone,
            )

        assertEquals(
            listOf("alpha", "bravo", "charlie"),
            rankLabelUsage(rows, LabelUsagePeriod.WEEK).map { it.label.name },
        )
    }

    @Test
    fun `a label whose work was finished in the window is in use, even if nothing was tagged`() {
        val finished = label("finished")
        val forgotten = label("forgotten")

        val rows =
            labelUsage(
                listOf(finished, forgotten),
                listOf(
                    tagging(
                        finished,
                        taggedAt = daysAgo(60),
                        status = TaskStatus.COMPLETED,
                        completedAt = daysAgo(1),
                    ),
                    tagging(forgotten, taggedAt = daysAgo(60)),
                ),
                now,
                zone,
            )

        val split = splitLabelUsage(rows, LabelUsagePeriod.WEEK)
        assertEquals(listOf("finished"), split.active.map { it.label.name })
        assertEquals(listOf("forgotten"), split.idle.map { it.label.name })
    }

    @Test
    fun `unused labels are offered most recently used first, never used last`() {
        val recent = label("recent")
        val older = label("older")
        val never = label("never")

        val rows =
            labelUsage(
                listOf(never, older, recent),
                listOf(
                    tagging(recent, taggedAt = daysAgo(20)),
                    tagging(older, taggedAt = daysAgo(60)),
                ),
                now,
                zone,
            )

        val split = splitLabelUsage(rows, LabelUsagePeriod.WEEK)
        assertEquals(emptyList(), split.active.map { it.label.name })
        assertEquals(listOf("recent", "older", "never"), split.idle.map { it.label.name })
    }

    @Test
    fun `the totals add up the window, except the overdue backlog which is current`() {
        val one = label("one")
        val two = label("two")
        val todayStart = ViewWindows.dayStart(now, zone)
        val longOverdue = todayStart - 200 * ViewWindows.DAY_MILLIS

        val rows =
            labelUsage(
                listOf(one, two),
                listOf(
                    tagging(one, taggedAt = daysAgo(1), dueAt = longOverdue),
                    tagging(
                        two,
                        taggedAt = daysAgo(1),
                        status = TaskStatus.COMPLETED,
                        completedAt = daysAgo(1),
                    ),
                ),
                now,
                zone,
            )

        val totals = labelUsageTotals(rows, LabelUsagePeriod.WEEK)
        assertEquals(2, totals.applied)
        assertEquals(1, totals.completed)
        assertEquals(1, totals.overdue, "the overdue backlog predates every window and is still counted")
    }

    @Test
    fun `the age of a use is counted in whole days where the user is`() {
        val yesterdayLateEvening =
            now.atZone(zone).toLocalDate().minusDays(1).atStartOfDay(zone)
                .plusHours(23).plusMinutes(59).toInstant().toEpochMilli()

        assertNull(lastUsedDaysAgo(null, now, zone))
        assertEquals(0L, lastUsedDaysAgo(now.toEpochMilli(), now, zone))
        assertEquals(1L, lastUsedDaysAgo(yesterdayLateEvening, now, zone))
    }
}
