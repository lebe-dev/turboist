package ru.tinyops.turboist.nativeapp.labels

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.nativeapp.quickadd.RecentProjects
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.BulkTaskActions
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import ru.tinyops.turboist.nativeapp.tasks.TaskListPresenter
import ru.tinyops.turboist.nativeapp.tasks.TaskListRepository
import ru.tinyops.turboist.nativeapp.tasks.TaskListViewModel
import ru.tinyops.turboist.nativeapp.tasks.plainSection
import java.time.Instant
import javax.inject.Inject

/**
 * The day the report's windows are cut against, and again each time it turns
 * over.
 *
 * Midnight of the current day rather than the current instant: every window ends
 * at the end of today, so the moment inside the day makes no difference to a
 * single boundary — and pinning it to midnight means the numbers change when the
 * day changes and at no other time, instead of on every tick of the clock.
 */
private fun DayClock.reportDays(): Flow<Instant> =
    days().distinctUntilChanged().map { it.atStartOfDay(zone).toInstant() }

/**
 * The label report: how often each label is being reached for, counted on the
 * device.
 *
 * Nothing is fetched. The report is a standing query over the replica like every
 * other screen, so it is there in a tunnel, and it moves the moment a task is
 * tagged or ticked off on this device rather than when a request comes back.
 */
@HiltViewModel
class LabelStatsViewModel
    @Inject
    constructor(
        repository: LabelsRepository,
        actions: LabelActions,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        private val today = clock.reportDays()

        // The windows are cut in the device's zone because the server's own
        // configured timezone is on neither sync payload; see `labelUsage`.
        val presenter =
            LabelStatsPresenter(
                scope = viewModelScope,
                usage = repository.observeUsage(today, clock.zone),
                today = today,
                zone = clock.zone,
                actions = actions,
                sync = sync,
            )
    }

/**
 * One label: the work carrying it, and the label itself.
 *
 * The work is a task list like any other, so it is read and acted on through the
 * shared list machinery — a row here behaves exactly as the same row behaves on
 * the day view. The list is one undivided block: a label cuts across dates,
 * projects and phases of the day, so there is no heading it could honestly be
 * grouped under.
 */
@HiltViewModel
class LabelTasksViewModel
    @Inject
    constructor(
        labels: LabelsRepository,
        tasks: TaskListRepository,
        taskActions: TaskListActions,
        bulk: BulkTaskActions,
        labelActions: LabelActions,
        recent: RecentProjects,
        sync: SyncScheduler,
        clock: DayClock,
    ) : TaskListViewModel(clock) {
        private val opened = MutableStateFlow<Long?>(null)

        @OptIn(ExperimentalCoroutinesApi::class)
        private val tagged = opened.filterNotNull().flatMapLatest { labels.observeTasks(it) }

        @OptIn(ExperimentalCoroutinesApi::class)
        private val label = opened.filterNotNull().flatMapLatest { labels.observeLabel(it) }

        private val sections =
            combine(tagged, tasks.observeProjectTitles()) { rows, titles ->
                listOf(plainSection(key = "label", tasks = rows, projectTitles = titles))
            }

        override val presenter =
            TaskListPresenter(
                viewModelScope,
                sections,
                taskActions,
                sync,
                bulk = bulk,
                moveOptions = tasks.observeMoveOptions(),
                recentProjects = recent.observe(),
            )

        /** The label itself: what it is called, how it is drawn, and what can be done to it. */
        val detail =
            LabelDetailPresenter(
                scope = viewModelScope,
                label = label,
                taggedTasks = tagged.map { it.size },
                actions = labelActions,
            )

        /**
         * Points the screen at a label, by the id this device holds it under.
         * Called once by the destination that knows it; pointing it at the same
         * label again changes nothing.
         */
        fun open(labelLocalId: Long) {
            opened.value = labelLocalId
        }
    }
