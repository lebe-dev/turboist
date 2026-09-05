package ru.tinyops.turboist.nativeapp.di

import android.content.Context
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.dao.ContextDao
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.LabelUsageDao
import ru.tinyops.turboist.core.database.dao.OutboxDao
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.ProjectSectionDao
import ru.tinyops.turboist.core.database.dao.SearchDao
import ru.tinyops.turboist.core.database.dao.SearchIndexDao
import ru.tinyops.turboist.core.database.dao.SettingsDao
import ru.tinyops.turboist.core.database.dao.SyncStateDao
import ru.tinyops.turboist.core.database.dao.TaskDao
import ru.tinyops.turboist.core.database.dao.TaskHydrationDao
import ru.tinyops.turboist.core.database.dao.TaskRelationDao
import ru.tinyops.turboist.core.database.dao.TaskTemplateDao
import ru.tinyops.turboist.core.database.dao.TaskViewDao
import javax.inject.Singleton

/**
 * The on-device replica, and the accessors onto it.
 *
 * One database instance for the process — Room keeps the write lock and the
 * invalidation tracker per instance, so a second one would see neither the
 * other's transactions nor its change notifications.
 */
@Module
@InstallIn(SingletonComponent::class)
object DatabaseModule {
    @Provides
    @Singleton
    fun database(
        @ApplicationContext context: Context,
    ): TurboistDatabase = TurboistDatabase.open(context)

    @Provides
    fun contextDao(database: TurboistDatabase): ContextDao = database.contexts()

    @Provides
    fun labelDao(database: TurboistDatabase): LabelDao = database.labels()

    @Provides
    fun labelUsageDao(database: TurboistDatabase): LabelUsageDao = database.labelUsage()

    @Provides
    fun projectDao(database: TurboistDatabase): ProjectDao = database.projects()

    @Provides
    fun sectionDao(database: TurboistDatabase): ProjectSectionDao = database.sections()

    @Provides
    fun taskDao(database: TurboistDatabase): TaskDao = database.tasks()

    @Provides
    fun taskRelationDao(database: TurboistDatabase): TaskRelationDao = database.taskRelations()

    @Provides
    fun taskViewDao(database: TurboistDatabase): TaskViewDao = database.taskViews()

    @Provides
    fun taskHydrationDao(database: TurboistDatabase): TaskHydrationDao = database.taskHydration()

    @Provides
    fun taskTemplateDao(database: TurboistDatabase): TaskTemplateDao = database.taskTemplates()

    @Provides
    fun settingsDao(database: TurboistDatabase): SettingsDao = database.settings()

    @Provides
    fun syncStateDao(database: TurboistDatabase): SyncStateDao = database.syncState()

    @Provides
    fun outboxDao(database: TurboistDatabase): OutboxDao = database.outbox()

    @Provides
    fun searchIndexDao(database: TurboistDatabase): SearchIndexDao = database.searchIndex()

    @Provides
    fun searchDao(database: TurboistDatabase): SearchDao = database.search()
}
