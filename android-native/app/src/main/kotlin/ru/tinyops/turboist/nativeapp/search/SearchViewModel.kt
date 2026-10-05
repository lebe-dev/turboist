package ru.tinyops.turboist.nativeapp.search

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import javax.inject.Inject

/**
 * The search screen's hold on the presenter.
 *
 * It carries nothing of its own. The behaviour lives in [SearchPresenter], which
 * needs neither Compose nor the platform, and this exists only to give it a
 * lifetime that survives a rotation.
 */
@HiltViewModel
class SearchViewModel
    @Inject
    constructor(
        repository: SearchRepository,
        recent: RecentSearches,
    ) : ViewModel() {
        val presenter = SearchPresenter(viewModelScope, repository, recent)
    }
