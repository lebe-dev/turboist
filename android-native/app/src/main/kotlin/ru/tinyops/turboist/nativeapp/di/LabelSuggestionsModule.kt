package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.labelsuggest.LabelSuggestions
import ru.tinyops.turboist.nativeapp.labelsuggest.OnDeviceLabelSuggestions
import javax.inject.Singleton

/**
 * The seam the capture sheet and the task screen ask for label suggestions
 * through, so a screen test runs on [LabelSuggestions.None] without a model.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class LabelSuggestionsModule {
    @Binds
    @Singleton
    abstract fun bindLabelSuggestions(suggestions: OnDeviceLabelSuggestions): LabelSuggestions
}
