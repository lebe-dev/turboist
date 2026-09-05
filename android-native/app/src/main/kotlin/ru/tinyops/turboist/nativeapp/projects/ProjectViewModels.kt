package ru.tinyops.turboist.nativeapp.projects

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.nativeapp.settings.SettingsRepository
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * How long a screen's queries stay open after it stops being watched. Long
 * enough to survive a rotation, short enough that a screen left behind stops
 * observing the replica.
 */
private const val STOP_TIMEOUT_MILLIS = 5_000L

/**
 * Whether the daily plan is part of this installation's product.
 *
 * Read from the replicated preference document rather than kept per device, so
 * a workspace that does not use the plan looks the same on every one of them. A
 * device that has not synced yet answers "off" and starts showing the plan the
 * moment the preference arrives.
 */
private fun SettingsRepository.dailyPlanEnabled(): Flow<Boolean> = observeUserSettings().map { it.troikiEnabled }

/** The workspace, browsed. */
@HiltViewModel
class ProjectsViewModel
    @Inject
    constructor(
        repository: ProjectsRepository,
        settings: SettingsRepository,
        actions: ProjectActions,
        sync: SyncScheduler,
    ) : ViewModel() {
        val presenter =
            ProjectsPresenter(
                scope = viewModelScope,
                groups = repository.observeGroups(),
                contexts = repository.observeContexts(),
                dailyPlanEnabled = settings.dailyPlanEnabled(),
                actions = actions,
                sync = sync,
            )
    }

/**
 * One project and its board.
 *
 * The project arrives from the destination rather than from the graph's saved
 * state, and is watched rather than read once: a project renamed on another
 * device, or deleted there, reaches this screen the moment the change is
 * replicated instead of leaving it showing something that is no longer true.
 */
@HiltViewModel
class ProjectBoardViewModel
    @Inject
    constructor(
        repository: ProjectsRepository,
        settings: SettingsRepository,
        actions: ProjectActions,
        taskActions: TaskListActions,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        private val opened = MutableStateFlow<Long?>(null)

        /** The zone every date on the screen is read in. */
        val zone: ZoneId = clock.zone

        /** The day the screen calls "today", and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        @OptIn(ExperimentalCoroutinesApi::class)
        private val content =
            opened.filterNotNull().flatMapLatest { projectLocalId -> repository.observeBoard(projectLocalId) }

        val presenter =
            ProjectBoardPresenter(
                viewModelScope,
                content,
                settings.dailyPlanEnabled(),
                actions,
                taskActions,
                sync,
            )

        /**
         * Points the screen at a project, by the id this device holds it under.
         * Called once by the destination that knows it; pointing it at the same
         * project again changes nothing.
         */
        fun open(projectLocalId: Long) {
            opened.value = projectLocalId
        }
    }

/** One context: the projects filed under it, and every task in the branch. */
@HiltViewModel
class ContextViewModel
    @Inject
    constructor(
        repository: ProjectsRepository,
        settings: SettingsRepository,
        actions: ContextActions,
        taskActions: TaskListActions,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        private val opened = MutableStateFlow<Long?>(null)

        /** The zone every date on the screen is read in. */
        val zone: ZoneId = clock.zone

        /** The day the screen calls "today", and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        @OptIn(ExperimentalCoroutinesApi::class)
        private val content =
            opened.filterNotNull().flatMapLatest { contextLocalId -> repository.observeContext(contextLocalId) }

        val presenter =
            ContextPresenter(viewModelScope, content, settings.dailyPlanEnabled(), actions, taskActions, sync)

        /** Points the screen at a context, by the id this device holds it under. */
        fun open(contextLocalId: Long) {
            opened.value = contextLocalId
        }
    }
