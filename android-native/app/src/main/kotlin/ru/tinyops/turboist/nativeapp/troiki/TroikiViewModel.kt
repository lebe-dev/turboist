package ru.tinyops.turboist.nativeapp.troiki

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import ru.tinyops.turboist.nativeapp.tasks.TaskListActions
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/** How long the plan's queries stay open after the screen stops being watched. */
private const val STOP_TIMEOUT_MILLIS = 5_000L

/** The daily plan: three buckets, the projects standing in them, and their work. */
@HiltViewModel
class TroikiViewModel
    @Inject
    constructor(
        repository: TroikiRepository,
        actions: TroikiActions,
        taskActions: TaskListActions,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        /** The zone every date on the screen is read in. */
        val zone: ZoneId = clock.zone

        /** The day the screen calls "today", and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        val presenter =
            TroikiPresenter(
                scope = viewModelScope,
                slots = repository.observeBoard(),
                assignable = repository.observeAssignable(),
                actions = actions,
                taskActions = taskActions,
                sync = sync,
            )
    }
