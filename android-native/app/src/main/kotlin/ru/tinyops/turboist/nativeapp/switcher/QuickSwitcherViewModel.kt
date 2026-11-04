package ru.tinyops.turboist.nativeapp.switcher

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import ru.tinyops.turboist.nativeapp.quickadd.QuickAddActions
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import javax.inject.Inject

/** The quick switcher's hold on its presenter, giving it a lifetime that survives a rotation. */
@HiltViewModel
class QuickSwitcherViewModel
    @Inject
    constructor(
        repository: QuickSwitcherRepository,
        recentItems: RecentItems,
        actions: QuickAddActions,
        clock: DayClock,
    ) : ViewModel() {
        val presenter =
            QuickSwitcherPresenter(
                scope = viewModelScope,
                repository = repository,
                recentItems = recentItems,
                actions = actions,
                zone = clock.zone,
                today = clock::today,
            )
    }
