package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.labels.LabelActions
import ru.tinyops.turboist.nativeapp.labels.WriteRepoLabelActions
import javax.inject.Singleton

/**
 * The one seam the label screens depend on.
 *
 * The screens ask for an interface so what "rename this label" does can be
 * replaced in a test without a screen changing — and so a screen test needs
 * neither a database nor a sync engine to run.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class LabelsModule {
    @Binds
    @Singleton
    abstract fun bindLabelActions(actions: WriteRepoLabelActions): LabelActions
}
