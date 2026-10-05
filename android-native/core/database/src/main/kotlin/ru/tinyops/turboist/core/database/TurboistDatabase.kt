package ru.tinyops.turboist.core.database

import android.content.Context
import androidx.room.Database
import androidx.room.Room
import androidx.room.RoomDatabase
import androidx.room.TypeConverters
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
import ru.tinyops.turboist.core.database.entity.AppSettingsRow
import ru.tinyops.turboist.core.database.entity.ContextFtsRow
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelFtsRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.entity.ProjectFtsRow
import ru.tinyops.turboist.core.database.entity.ProjectLabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.ProjectSectionRow
import ru.tinyops.turboist.core.database.entity.QuarantinedOpRow
import ru.tinyops.turboist.core.database.entity.SyncStateRow
import ru.tinyops.turboist.core.database.entity.TaskFtsRow
import ru.tinyops.turboist.core.database.entity.TaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskRelationRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateLabelRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskLabelRow
import ru.tinyops.turboist.core.database.entity.TaskTemplateSubtaskRow
import ru.tinyops.turboist.core.database.entity.UserSettingsRow
import ru.tinyops.turboist.core.database.entity.UserStateRow

/**
 * The on-device replica.
 *
 * This database is what the screens read. Nothing renders from a response: a
 * request updates the replica and the replica updates the screen, which is what
 * makes every screen work with no network and makes "loading" a property of the
 * data being absent rather than of a request being in flight.
 *
 * It holds three kinds of table, and they are not the same thing:
 *
 * - the workspace tables, which are a copy of records the server owns;
 * - the full-text indexes, which are derived from those tables and maintained by
 *   triggers, so they can always be thrown away and rebuilt;
 * - the bookkeeping tables, which exist only here. They are how far the copy has
 *   caught up, what this device has changed and not yet sent, and what the
 *   server refused. Nothing in them is ever uploaded as data.
 *
 * The schema is versioned independently of the server's. A migration here
 * reshapes a copy; the copy can always be discarded and fetched again, which is
 * a freedom the server's own migrations do not have.
 */
@Database(
    entities = [
        // The replicated workspace.
        ContextRow::class,
        LabelRow::class,
        ProjectRow::class,
        ProjectLabelRow::class,
        ProjectSectionRow::class,
        TaskRow::class,
        TaskLabelRow::class,
        TaskRelationRow::class,
        TaskTemplateRow::class,
        TaskTemplateSubtaskRow::class,
        TaskTemplateLabelRow::class,
        TaskTemplateSubtaskLabelRow::class,
        UserSettingsRow::class,
        AppSettingsRow::class,
        UserStateRow::class,
        // Derived from the tables above.
        TaskFtsRow::class,
        ProjectFtsRow::class,
        LabelFtsRow::class,
        ContextFtsRow::class,
        // Local-only bookkeeping.
        SyncStateRow::class,
        OutboxOpRow::class,
        QuarantinedOpRow::class,
    ],
    version = TurboistDatabase.VERSION,
    exportSchema = true,
)
@TypeConverters(ReplicaConverters::class)
abstract class TurboistDatabase : RoomDatabase() {
    abstract fun contexts(): ContextDao

    abstract fun labels(): LabelDao

    /** Every tagging with its task's facts, which the label usage report is counted from. */
    abstract fun labelUsage(): LabelUsageDao

    abstract fun projects(): ProjectDao

    abstract fun sections(): ProjectSectionDao

    abstract fun tasks(): TaskDao

    abstract fun taskRelations(): TaskRelationDao

    /** The list queries every screen reads through. */
    abstract fun taskViews(): TaskViewDao

    /** The labels, relation edges and parent links a rendered list needs beside its rows. */
    abstract fun taskHydration(): TaskHydrationDao

    abstract fun taskTemplates(): TaskTemplateDao

    abstract fun settings(): SettingsDao

    abstract fun syncState(): SyncStateDao

    abstract fun outbox(): OutboxDao

    abstract fun searchIndex(): SearchIndexDao

    /** Search itself: the index paired with the rows it names, in ranked order. */
    abstract fun search(): SearchDao

    companion object {
        const val VERSION: Int = 1

        /** The file the replica lives in, inside the app's private storage. */
        const val FILE_NAME: String = "turboist-replica.db"

        /**
         * Opens the replica.
         *
         * There is one builder, here, rather than one per caller: the settings
         * below are properties of the replica itself, and a second caller
         * configuring its own would be opening a subtly different database.
         */
        fun open(context: Context): TurboistDatabase =
            Room.databaseBuilder(context.applicationContext, TurboistDatabase::class.java, FILE_NAME)
                .build()
    }
}
