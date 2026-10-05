package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.templates.TemplateActions
import ru.tinyops.turboist.nativeapp.templates.TemplateCapture
import ru.tinyops.turboist.nativeapp.templates.WriteRepoTemplateActions
import javax.inject.Singleton

/**
 * The seam the template surfaces depend on.
 *
 * They ask for an interface so that what saving or using a template does can be
 * replaced in a test without a screen changing — and so a test of one needs no
 * database.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class TemplatesModule {
    @Binds
    @Singleton
    abstract fun bindTemplateActions(actions: WriteRepoTemplateActions): TemplateActions

    /**
     * The same object under a narrower name, for the task screen: cutting a
     * template out of a task is the one thing it does with templates, and a
     * screen that asked for the whole catalogue could reach further than that.
     */
    @Binds
    @Singleton
    abstract fun bindTemplateCapture(actions: WriteRepoTemplateActions): TemplateCapture
}
