package ru.tinyops.turboist.nativeapp.quickadd

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import java.time.LocalDate
import java.time.ZoneId
import javax.inject.Inject

/**
 * The capture surface, wired to the app's graph.
 *
 * It holds nothing of its own beyond the requests coming in from outside: the
 * behaviour is in the presenter, so the sheet can be driven in a check without a
 * database, a clock or a dependency graph behind it.
 */
@HiltViewModel
class QuickAddViewModel
    @Inject
    constructor(
        repository: QuickAddRepository,
        recent: RecentProjects,
        actions: QuickAddActions,
        requests: QuickAddRequests,
        clock: DayClock,
    ) : ViewModel() {
        /** The zone the day shortcuts are read in. */
        val zone: ZoneId = clock.zone

        /** The day "today" means, and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        val presenter = QuickAddPresenter(viewModelScope, repository.observeWorkspace(), recent, actions, clock.zone)

        init {
            // A share or a launcher shortcut can arrive long before this exists —
            // during sign-in, or while the app was not running at all. It waits
            // where the activity left it and opens the sheet as soon as there is
            // one to open.
            viewModelScope.launch {
                requests.pending().collect { request ->
                    presenter.open(request)
                    requests.consumed()
                }
            }
        }

        private companion object {
            /** Matches the presenter's own grace period, so both stop watching together. */
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }
