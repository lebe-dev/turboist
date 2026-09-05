package ru.tinyops.turboist.nativeapp.unsent

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

/** What this device is holding back, and what to do about the part of it that is stuck. */
@HiltViewModel
class UnsentChangesViewModel
    @Inject
    constructor(
        repository: UnsentChangesRepository,
    ) : ViewModel() {
        val presenter =
            UnsentChangesPresenter(
                scope = viewModelScope,
                setAside = repository.observeSetAside(),
                waiting = repository.observeWaiting(),
                actions = repository,
            )
    }
