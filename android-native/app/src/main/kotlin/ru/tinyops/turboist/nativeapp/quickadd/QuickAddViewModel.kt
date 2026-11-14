package ru.tinyops.turboist.nativeapp.quickadd

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.nativeapp.labels.RecentLabels
import ru.tinyops.turboist.nativeapp.labelsuggest.LabelSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.ProjectSuggestions
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
        recentLabels: RecentLabels,
        activity: ProjectActivity,
        actions: QuickAddActions,
        private val requests: QuickAddRequests,
        clock: DayClock,
        labelSuggestions: LabelSuggestions,
        projectSuggestions: ProjectSuggestions,
    ) : ViewModel() {
        /** The zone the day shortcuts are read in. */
        val zone: ZoneId = clock.zone

        /** The day "today" means, and again when it turns over. */
        val today: StateFlow<LocalDate> =
            clock.days().stateIn(viewModelScope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), clock.today())

        val presenter =
            QuickAddPresenter(
                viewModelScope,
                repository.observeWorkspace(),
                recent,
                actions,
                clock.zone,
                recentLabels,
                clock::now,
                activity = activity,
                labelSuggestions = labelSuggestions,
                projectSuggestions = projectSuggestions,
            )

        /**
         * Opens the sheet for every capture asked for from outside, until cancelled.
         *
         * A share or a launcher shortcut can arrive long before this is called —
         * during sign-in, or while the app was not running at all — and waits
         * where the activity left it. It is meant to be run only while the screen
         * is on top: a browser may start a second instance of the activity for
         * each share, and an instance nobody is looking at must not take the
         * request and open the sheet where it cannot be seen.
         */
        suspend fun serveRequests() {
            requests.pending().collect { request ->
                presenter.open(request)
                requests.consumed()
            }
        }

        private companion object {
            /** Matches the presenter's own grace period, so both stop watching together. */
            const val STOP_TIMEOUT_MILLIS = 5_000L
        }
    }
