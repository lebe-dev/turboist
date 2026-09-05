package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.projects.ContextActions
import ru.tinyops.turboist.nativeapp.projects.ProjectActions
import ru.tinyops.turboist.nativeapp.projects.WriteRepoContextActions
import ru.tinyops.turboist.nativeapp.projects.WriteRepoProjectActions
import javax.inject.Singleton

/**
 * The seams the project and context screens depend on.
 *
 * The screens ask for interfaces so that what a board does when a column is
 * dragged, or a project is finished, can be replaced in a test without a screen
 * changing — and so a test of a screen needs no database.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class ProjectsModule {
    @Binds
    @Singleton
    abstract fun bindProjectActions(actions: WriteRepoProjectActions): ProjectActions

    /**
     * A separate port from the projects one: a context screen creates, renames
     * and deletes contexts and does nothing to a board, so it depends on those
     * three writes rather than on the whole catalogue.
     */
    @Binds
    @Singleton
    abstract fun bindContextActions(actions: WriteRepoContextActions): ContextActions
}
