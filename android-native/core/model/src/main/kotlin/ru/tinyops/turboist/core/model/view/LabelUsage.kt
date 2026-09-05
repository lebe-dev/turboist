package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.TaskStatus
import java.time.Instant
import java.time.ZoneId

/**
 * How far back one column of the label usage report looks.
 *
 * The windows are **rolling** — the last N days ending tonight — rather than
 * calendar-aligned. The report answers "how often am I reaching for this label
 * lately", and a calendar week that is one day old would make every label look
 * unused on a Tuesday morning. Rolling windows also make the comparison against
 * the window before them symmetric: two spans of exactly the same length.
 */
enum class LabelUsagePeriod(val days: Int) {
    WEEK(7),
    MONTH(30),
    QUARTER(90),
}

/**
 * One task↔label tagging, with the facts about the task carrying it.
 *
 * The tagging's own moment is the report's whole basis: it is when the label was
 * *applied*, which is a different event from when the task was created and from
 * when it was finished. It is nullable because a tagging can predate the moment
 * being recorded at all; such an edge counts in the all-time totals and in no
 * window, which is more honest than pretending it happened at the epoch.
 */
data class LabelTagging(
    val labelLocalId: Long,
    val taggedAt: Long?,
    val status: TaskStatus,
    val dueAt: Long?,
    val completedAt: Long?,
    val projectLocalId: Long?,
)

/**
 * One window's counters for a single label.
 *
 * [applied] counts taggings made inside the window; [completed] counts tasks
 * carrying the label that were finished inside it. The two are deliberately
 * independent: a task tagged months ago but finished this week is this week's
 * completion and nobody's application.
 *
 * [previousApplied] is the same count over the equally long window immediately
 * before this one, which is what the trend beside a row is read from.
 */
data class LabelPeriodUsage(
    val applied: Int = 0,
    val previousApplied: Int = 0,
    val completed: Int = 0,
) {
    /** How much more, or less, the label was reached for than in the window before. */
    val delta: Int get() = applied - previousApplied
}

/**
 * Everything the usage report knows about one label.
 *
 * The three period counters travel together rather than one report per window,
 * so switching the period on screen is a re-read of numbers already in hand
 * instead of another pass over the replica.
 *
 * [totalTasks], [openTasks], [overdue] and [projects] are period-independent on
 * purpose: they describe the label as it stands now, not what happened inside a
 * window, and they must not move when the user switches period.
 */
data class LabelUsage(
    val label: Label,
    val totalTasks: Int = 0,
    val openTasks: Int = 0,
    val overdue: Int = 0,
    val projects: Int = 0,
    val lastUsedAt: Long? = null,
    val periods: Map<LabelUsagePeriod, LabelPeriodUsage> = emptyMap(),
) {
    /** This label's counters for one window; all zeroes for a label never used in it. */
    fun period(period: LabelUsagePeriod): LabelPeriodUsage = periods[period] ?: LabelPeriodUsage()
}

/** The active rows of a report, ranked, and the ones with nothing to show. */
data class LabelUsageRanking(
    val active: List<LabelUsage>,
    val idle: List<LabelUsage>,
)

/**
 * The headline counters above a report.
 *
 * [applied] and [completed] belong to the chosen window. [overdue] does not — it
 * is the past-due open work carried under these labels right now, which is why
 * it stays put when the period changes.
 */
data class LabelUsageTotals(
    val applied: Int = 0,
    val completed: Int = 0,
    val overdue: Int = 0,
)

/**
 * The window one period covers: the last [LabelUsagePeriod.days] days, ending at
 * the end of the day [now] falls on.
 *
 * It ends at tomorrow's midnight rather than at this instant so today counts in
 * full — a label applied an hour ago and one applied this morning are both
 * "today", and a report read at nine would otherwise show less than the same
 * report read at five.
 */
fun labelUsageWindow(
    now: Instant,
    zone: ZoneId,
    period: LabelUsagePeriod,
): TimeWindow = ViewWindows.completedHistory(now, zone, period.days)

/** The equally long window immediately before [window], which the trend compares against. */
fun previousWindow(window: TimeWindow): TimeWindow =
    TimeWindow(window.from - (window.untilExclusive - window.from), window.from)

/**
 * The usage report, computed over the replica.
 *
 * Every label is returned, the never-used ones with zeroes: the screen offers a
 * cleanup list, and a label absent from the report could not appear on it.
 *
 * The zone decides where every window boundary falls, so it has to be the zone
 * the rest of the product measures days in. Reading the same replica in two
 * different zones shifts the boundaries by hours and moves taggings between
 * windows without anything on screen saying so.
 *
 * The server cuts the same windows in its own configured timezone, but that
 * setting reaches neither sync payload — not the snapshot, not the change feed —
 * so the device cannot use it. Callers pass the device's zone instead, a
 * deliberate substitute and the same zone every other dated screen here reads.
 * The two agree while the phone is where the server thinks it is, and differ by
 * the offset between them otherwise, which can move a tagging made near midnight
 * into the neighbouring window.
 *
 * A task is counted as finished work only while its status still says so. The
 * server clears the completion moment when a task is cancelled, so the two agree
 * on server-shaped rows; asking about the status as well is what keeps a
 * locally cancelled task — whose stale moment the device has not cleared — out
 * of the finished count, because abandoning work is not finishing it.
 */
fun labelUsage(
    labels: List<Label>,
    taggings: Collection<LabelTagging>,
    now: Instant,
    zone: ZoneId,
): List<LabelUsage> {
    val todayStart = ViewWindows.dayStart(now, zone)
    val windows = LabelUsagePeriod.entries.associateWith { labelUsageWindow(now, zone, it) }
    val byLabel = taggings.groupBy { it.labelLocalId }
    return labels.map { label -> usageOf(label, byLabel[label.localId].orEmpty(), windows, todayStart) }
}

private fun usageOf(
    label: Label,
    taggings: List<LabelTagging>,
    windows: Map<LabelUsagePeriod, TimeWindow>,
    todayStart: Long,
): LabelUsage =
    LabelUsage(
        label = label,
        totalTasks = taggings.size,
        openTasks = taggings.count { it.status == TaskStatus.OPEN },
        overdue = taggings.count { it.status == TaskStatus.OPEN && it.dueAt != null && it.dueAt < todayStart },
        projects = taggings.mapNotNullTo(HashSet()) { it.projectLocalId }.size,
        lastUsedAt = taggings.mapNotNull { it.taggedAt }.maxOrNull(),
        periods = windows.mapValues { (_, window) -> countIn(taggings, window) },
    )

private fun countIn(
    taggings: List<LabelTagging>,
    window: TimeWindow,
): LabelPeriodUsage {
    val previous = previousWindow(window)
    return LabelPeriodUsage(
        applied = taggings.count { it.taggedAt != null && it.taggedAt in window },
        previousApplied = taggings.count { it.taggedAt != null && it.taggedAt in previous },
        completed =
            taggings.count {
                it.status == TaskStatus.COMPLETED && it.completedAt != null && it.completedAt in window
            },
    )
}

/**
 * The report as the ranking reads it: most applications first, then most
 * completions, then by name.
 *
 * The name is the last word so the order is total — two labels reached for the
 * same number of times must not swap places between one redraw and the next,
 * which they would if the engine were left to choose.
 */
fun rankLabelUsage(
    rows: List<LabelUsage>,
    period: LabelUsagePeriod,
): List<LabelUsage> =
    rows.sortedWith(
        compareByDescending<LabelUsage> { it.period(period).applied }
            .thenByDescending { it.period(period).completed }
            .thenBy { it.label.name.lowercase() },
    )

/**
 * True when the label was reached for, or work carrying it was finished, inside
 * the window.
 *
 * Completions count on their own: finishing something tagged months ago is still
 * activity under that label, and treating it as idle would offer live work up
 * for cleanup.
 */
fun isActiveIn(
    row: LabelUsage,
    period: LabelUsagePeriod,
): Boolean = row.period(period).applied > 0 || row.period(period).completed > 0

/**
 * Splits a report into what is in use and what is not.
 *
 * The active rows keep the ranking order. The idle ones are ordered by how
 * recently they were last reached for, most recent first and never-used last —
 * that is the order somebody clearing labels out works in.
 */
fun splitLabelUsage(
    rows: List<LabelUsage>,
    period: LabelUsagePeriod,
): LabelUsageRanking {
    val ranked = rankLabelUsage(rows, period)
    return LabelUsageRanking(
        active = ranked.filter { isActiveIn(it, period) },
        idle =
            ranked
                .filterNot { isActiveIn(it, period) }
                .sortedWith(compareBy(nullsLast(reverseOrder())) { it.lastUsedAt }),
    )
}

/** The headline counters for one window over the whole report. */
fun labelUsageTotals(
    rows: List<LabelUsage>,
    period: LabelUsagePeriod,
): LabelUsageTotals =
    LabelUsageTotals(
        applied = rows.sumOf { it.period(period).applied },
        completed = rows.sumOf { it.period(period).completed },
        overdue = rows.sumOf { it.overdue },
    )

/**
 * Whole-day age of a label's most recent application, in the zone days are
 * measured in: 0 is today, 1 is yesterday, null is never used.
 *
 * Counted between calendar days rather than by dividing elapsed milliseconds, so
 * a tagging made at one minute to midnight reads as "yesterday" the next morning
 * instead of as "today".
 */
fun lastUsedDaysAgo(
    lastUsedAt: Long?,
    now: Instant,
    zone: ZoneId,
): Long? {
    if (lastUsedAt == null) return null
    val used = Instant.ofEpochMilli(lastUsedAt).atZone(zone).toLocalDate()
    val today = now.atZone(zone).toLocalDate()
    return maxOf(0L, used.toEpochDay().let { today.toEpochDay() - it })
}
