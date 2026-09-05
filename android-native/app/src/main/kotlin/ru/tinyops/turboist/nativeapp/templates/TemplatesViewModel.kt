package ru.tinyops.turboist.nativeapp.templates

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import javax.inject.Inject

/** The reusable blueprints: kept, edited, and dropped into a project. */
@HiltViewModel
class TemplatesViewModel
    @Inject
    constructor(
        repository: TemplateRepository,
        actions: TemplateActions,
        sync: SyncScheduler,
    ) : ViewModel() {
        val presenter =
            TemplatesPresenter(
                scope = viewModelScope,
                templates = repository.observeTemplates(),
                projects = repository.observeProjects(),
                knownLabels = repository.observeLabels(),
                actions = actions,
                sync = sync,
            )
    }
