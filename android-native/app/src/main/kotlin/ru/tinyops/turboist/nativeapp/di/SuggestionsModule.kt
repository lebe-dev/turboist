package ru.tinyops.turboist.nativeapp.di

import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.labelsuggest.LabelSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.OnDeviceSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.ProjectSuggestions

/**
 * The seams the capture sheet and the task screen ask for suggestions through,
 * so a screen test runs on [LabelSuggestions.None] and [ProjectSuggestions.None]
 * without a model. Both are faces of the one on-device index.
 */
@Module
@InstallIn(SingletonComponent::class)
object SuggestionsModule {
    @Provides
    fun labelSuggestions(suggestions: OnDeviceSuggestions): LabelSuggestions = suggestions.labels

    @Provides
    fun projectSuggestions(suggestions: OnDeviceSuggestions): ProjectSuggestions = suggestions.projects
}
