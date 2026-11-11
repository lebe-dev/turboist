package ru.tinyops.turboist.nativeapp.todaylock

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.sync.write.SettingsWriteRepo
import ru.tinyops.turboist.nativeapp.settings.SettingsRepository
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import javax.inject.Inject

/** The Today lock control's state, held across recompositions of the top bar. */
@HiltViewModel
class TodayLockViewModel
    @Inject
    constructor(
        settings: SettingsRepository,
        clock: DayClock,
        writes: SettingsWriteRepo,
    ) : ViewModel() {
        val presenter: TodayLockPresenter =
            TodayLockPresenter(
                scope = viewModelScope,
                lockUntil = settings.observeUserSettings().map { it.todayLockUntil },
                now = clock.ticks(),
                lockToday = { writes.lockToday() },
            )
    }
