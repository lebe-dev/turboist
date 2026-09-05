package ru.tinyops.turboist.nativeapp.tasks

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.view.TimeWindow
import ru.tinyops.turboist.core.model.view.ViewWindows
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.calendar.CalendarRepository
import ru.tinyops.turboist.nativeapp.quickadd.RecentProjects
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * What every task list screen is, whatever query stands behind it.
 *
 * The screen reads three things from here and nothing else: the rows, the zone
 * the user's days are measured in, and which day counts as today. The last two
 * are here rather than in the presenter because they are not state the screen
 * changes — they are the frame a due date is read against, and a list left open
 * over midnight has to be handed the new one.
 */
abstract class TaskListViewModel(
    clock: DayClock,
) : ViewModel() {
    /** The zone every date on the screen is read in. */
    val zone: ZoneId = clock.zone

    /** The day the screen calls "today", and again when it turns over. */
    val today: StateFlow<LocalDate> =
        clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

    /** The rows, the writes and the refresh gesture. */
    abstract val presenter: TaskListPresenter

    private companion object {
        /** Matches the presenter's own grace period, so both stop watching together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

/**
 * The day the user is in, and which phase of it is happening now.
 *
 * The two travel together because a screen that re-read one without the other
 * could draw yesterday's list with today's highlight for up to a minute.
 */
private data class DayMoment(
    val dayStart: Long,
    val activePart: DayPart?,
)

/**
 * The week on screen, and the day inside it that counts as today.
 *
 * The day is kept alongside the window because a week view left open across
 * midnight must move the word "Today" onto the next heading, and the window
 * itself does not change when that happens.
 */
private data class WeekMoment(
    val dayStart: Long,
    val window: TimeWindow,
)

@OptIn(ExperimentalCoroutinesApi::class)
private fun DayClock.moments(schedule: DayPartSchedule): Flow<DayMoment> =
    ticks()
        .map { DayMoment(ViewWindows.dayStart(it, zone), schedule.activeAt(it, zone)) }
        .distinctUntilChanged()

/** Today: what has slipped past its date, then the day itself, phase by phase. */
@HiltViewModel
class TodayViewModel
    @Inject
    constructor(
        repository: TaskListRepository,
        actions: TaskListActions,
        bulk: BulkTaskActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        calendar: CalendarRepository,
        private val clock: DayClock,
    ) : TaskListViewModel(clock) {
        @OptIn(ExperimentalCoroutinesApi::class)
        private val sections =
            clock.moments(DayPartSchedule.Default).flatMapLatest { moment ->
                combine(
                    repository.observeOverdue(moment.dayStart),
                    repository.observeToday(moment.dayStart),
                    repository.observeProjectTitles(),
                    calendar.observe(ViewWindows.day(moment.dayStart)),
                ) { overdue, today, titles, events ->
                    val blocks =
                        listOfNotNull(overdueSection(overdue, titles)) +
                            dayPartSections(today, titles, moment.activePart)
                    withPhaseEvents(blocks, events, clock.zone, DayPartSchedule.Default, moment.activePart)
                }
            }

        @OptIn(ExperimentalCoroutinesApi::class)
        private val announcement =
            combine(
                repository.observeUserSettings(),
                clock.ticks().map { DayPartSchedule.Default.activeAt(it, clock.zone) }.distinctUntilChanged(),
            ) { settings, active -> todayAnnouncement(settings, active) }

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                actions,
                sync,
                announcement,
                calendar.cachedAsOf,
                bulk,
                repository.observeMoveOptions(),
                recent.observe(),
            )
    }

/** Tomorrow: the next day, phase by phase. Nothing is overdue in the future. */
@HiltViewModel
class TomorrowViewModel
    @Inject
    constructor(
        repository: TaskListRepository,
        actions: TaskListActions,
        bulk: BulkTaskActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        calendar: CalendarRepository,
        private val clock: DayClock,
    ) : TaskListViewModel(clock) {
        @OptIn(ExperimentalCoroutinesApi::class)
        private val sections =
            clock.moments(DayPartSchedule.Default).flatMapLatest { moment ->
                combine(
                    repository.observeTomorrow(moment.dayStart),
                    repository.observeProjectTitles(),
                    calendar.observe(ViewWindows.day(moment.dayStart + ViewWindows.DAY_MILLIS)),
                ) { tasks, titles, events ->
                    // Tomorrow has no phase happening yet, so no phase is
                    // highlighted: marking one would claim a time that has not
                    // arrived.
                    val blocks = dayPartSections(tasks, titles, activePart = null)
                    withPhaseEvents(blocks, events, clock.zone, DayPartSchedule.Default, activePart = null)
                }
            }

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                actions,
                sync,
                calendarAsOf = calendar.cachedAsOf,
                bulk = bulk,
                moveOptions = repository.observeMoveOptions(),
                recentProjects = recent.observe(),
            )
    }

/**
 * The week, day by day.
 *
 * The list is cut on calendar days rather than on phases: a week is read as
 * "when", and the phase a task is meant for only becomes a useful division once
 * the day it belongs to is the day happening now.
 */
@HiltViewModel
class WeekViewModel
    @Inject
    constructor(
        repository: TaskListRepository,
        actions: TaskListActions,
        bulk: BulkTaskActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        calendar: CalendarRepository,
        private val clock: DayClock,
    ) : TaskListViewModel(clock) {
        @OptIn(ExperimentalCoroutinesApi::class)
        private val sections =
            clock.ticks()
                .map { WeekMoment(ViewWindows.dayStart(it, clock.zone), ViewWindows.week(it, clock.zone)) }
                .distinctUntilChanged()
                .flatMapLatest { moment ->
                    combine(
                        repository.observeWeek(moment.window),
                        repository.observeProjectTitles(),
                        calendar.observe(moment.window),
                    ) { tasks, titles, events ->
                        val today = dayOf(moment.dayStart)
                        withDayEvents(dueDaySections(tasks, titles, clock.zone, today), events, clock.zone, today)
                    }
                }

        /** The calendar day the week view calls "today" when it names a heading. */
        private fun dayOf(dayStart: Long) = Instant.ofEpochMilli(dayStart).atZone(clock.zone).toLocalDate()

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                actions,
                sync,
                calendarAsOf = calendar.cachedAsOf,
                bulk = bulk,
                moveOptions = repository.observeMoveOptions(),
                recentProjects = recent.observe(),
            )
    }

/**
 * The planning view: what is parked, and what has been committed to the week.
 *
 * Both blocks are drawn even when empty. They are the two ends of one decision,
 * and a block that vanished when its last task left would take the place the
 * user moves work *to* off the screen.
 */
@HiltViewModel
class NextWeekViewModel
    @Inject
    constructor(
        repository: TaskListRepository,
        actions: TaskListActions,
        bulk: BulkTaskActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        clock: DayClock,
    ) : TaskListViewModel(clock) {
        @OptIn(ExperimentalCoroutinesApi::class)
        private val sections =
            clock.ticks()
                .map { ViewWindows.week(it, clock.zone) }
                .distinctUntilChanged()
                .flatMapLatest { window ->
                    combine(
                        repository.observeBacklog(),
                        repository.observePlannedForWeek(window),
                        repository.observeProjectTitles(),
                    ) { backlog, planned, titles ->
                        listOf(
                            namedSection(
                                key = "backlog",
                                titleRes = R.string.page_nextWeek_backlogTitle,
                                tasks = backlog,
                                projectTitles = titles,
                                emptyRes = R.string.page_nextWeek_backlogEmptyDesc,
                            ),
                            namedSection(
                                key = "planned",
                                titleRes = R.string.page_nextWeek_nextWeekTitle,
                                tasks = planned,
                                projectTitles = titles,
                                emptyRes = R.string.page_nextWeek_weekEmptyDesc,
                            ),
                        )
                    }
                }

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                actions,
                sync,
                bulk = bulk,
                moveOptions = repository.observeMoveOptions(),
                recentProjects = recent.observe(),
            )
    }
