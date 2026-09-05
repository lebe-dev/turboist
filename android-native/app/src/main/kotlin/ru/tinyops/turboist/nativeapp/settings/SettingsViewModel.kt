package ru.tinyops.turboist.nativeapp.settings

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

/** The three stores a settings screen sits over, kept apart. */
@HiltViewModel
class SettingsViewModel
    @Inject
    constructor(
        repository: SettingsRepository,
        deviceOptions: DeviceOptionsStore,
        actions: SettingsActions,
        device: DeviceOptionActions,
        connection: ServerConnection,
        release: AppRelease,
    ) : ViewModel() {
        val presenter =
            SettingsPresenter(
                scope = viewModelScope,
                userSettings = repository.observeUserSettings(),
                appSettings = repository.observeAppSettings(),
                labels = repository.observeLabels(),
                projects = repository.observeProjects(),
                deviceOptions = deviceOptions.observe(),
                actions = actions,
                device = device,
                connection = connection,
                release = release,
            )
    }
