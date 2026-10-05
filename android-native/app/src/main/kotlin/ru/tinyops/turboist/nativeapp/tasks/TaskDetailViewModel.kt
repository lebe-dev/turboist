package ru.tinyops.turboist.nativeapp.tasks

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.filterNotNull
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.templates.TemplateCapture
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * One task, opened at an address.
 *
 * The address arrives from the destination rather than from the graph's saved
 * state so that the same screen serves both ways in: a tap inside the app, which
 * already knows the device's own id, and a link from outside, which carries the
 * server's. Resolving the second one is a standing query, so a link opened a
 * moment before its task has been replicated finds it when it arrives instead of
 * failing once.
 */
@HiltViewModel
class TaskDetailViewModel
    @Inject
    constructor(
        repository: TaskDetailRepository,
        actions: TaskDetailActions,
        relationSearch: TaskRelationSearch,
        templates: TemplateCapture,
        sync: SyncScheduler,
        clock: DayClock,
    ) : ViewModel() {
        private val opened = MutableStateFlow<TaskAddress?>(null)

        /** The zone every date on the screen is read in. */
        val zone: ZoneId = clock.zone

        /** The day the screen calls "today", and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        @OptIn(ExperimentalCoroutinesApi::class)
        private val content =
            opened
                .filterNotNull()
                .flatMapLatest { address -> repository.observeLocalId(address) }
                .flatMapLatest { localId ->
                    if (localId == null) flowOf(null) else repository.observeDetail(localId)
                }

        /** The rows, the writes and the refresh gesture. */
        val presenter = TaskDetailPresenter(viewModelScope, content, actions, sync, relationSearch, templates)

        /** Where the task can be moved to. Read only while the move sheet is open. */
        val moveOptions: StateFlow<List<MoveProject>> =
            repository
                .observeMoveOptions()
                .stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), emptyList())

        /**
         * Points the screen at a task. Called once by the destination that knows
         * the address; pointing it at the same task again changes nothing.
         */
        fun open(address: TaskAddress) {
            opened.value = address
        }

        private companion object {
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }
