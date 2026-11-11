package ru.tinyops.turboist.nativeapp.quickadd

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

/** How many projects keep a time of last activity. More than a day's plan ever holds. */
const val PROJECT_ACTIVITY_MEMORY: Int = 50

/**
 * Records that a project was just worked with, at [at] in epoch milliseconds.
 *
 * Only the latest moment per project is kept, and only for the
 * [limit] projects touched most recently. Pure, because the cap and the
 * "latest wins" rule are what have to stay true.
 */
fun withProjectActivity(
    existing: Map<Long, Long>,
    projectLocalId: Long,
    at: Long,
    limit: Int = PROJECT_ACTIVITY_MEMORY,
): Map<Long, Long> {
    if (limit <= 0) return emptyMap()
    val latest = maxOf(at, existing[projectLocalId] ?: Long.MIN_VALUE)
    return (existing + (projectLocalId to latest))
        .entries
        .sortedByDescending { it.value }
        .take(limit)
        .associate { it.key to it.value }
}

/**
 * [items] with the projects worked with most recently first.
 *
 * The sort is stable and anything never touched keeps its place after the rest,
 * so a fresh device shows the list in the order it was given.
 */
fun <T> byRecentActivity(
    items: List<T>,
    activity: Map<Long, Long>,
    localIdOf: (T) -> Long,
): List<T> = items.sortedByDescending { activity[localIdOf(it)] ?: Long.MIN_VALUE }

/** The stored form: `12:1767225600000,3:1767222000000`. An entry that cannot be read is dropped. */
internal fun encodeProjectActivity(activity: Map<Long, Long>): String =
    activity.entries.joinToString(",") { "${it.key}:${it.value}" }

internal fun decodeProjectActivity(stored: String?): Map<Long, Long> =
    stored
        .orEmpty()
        .split(",")
        .mapNotNull { entry ->
            val id = entry.substringBefore(":", "").toLongOrNull()
            val at = entry.substringAfter(":", "").toLongOrNull()
            if (id == null || at == null) null else id to at
        }.toMap()

/**
 * When this device last worked with each project: opened it, opened one of its
 * tasks, or added a task to it.
 *
 * It orders the row of today's projects in the capture sheet, so the project
 * the user was just in is the one under their thumb. Device-local, like the
 * other recents: a habit of this phone, never synced, and kept by local id so a
 * project made offline counts like any other.
 */
interface ProjectActivity {
    /** Project local id to the last moment it was worked with, and again whenever that changes. */
    fun observe(): Flow<Map<Long, Long>>

    /** Records that a project was worked with at [at], in epoch milliseconds. */
    suspend fun touch(
        projectLocalId: Long,
        at: Long,
    )

    /** Forgets all of it. */
    suspend fun clear()

    /** Remembers nothing: for surfaces and checks that have no use for the order. */
    object None : ProjectActivity {
        override fun observe(): Flow<Map<Long, Long>> = flowOf(emptyMap())

        override suspend fun touch(
            projectLocalId: Long,
            at: Long,
        ) = Unit

        override suspend fun clear() = Unit
    }
}

/** Its own file: a process may open a given DataStore file only once. */
private val Context.projectActivityPreferences: DataStore<Preferences> by
    preferencesDataStore(name = "project_activity")

private val PROJECT_ACTIVITY = stringPreferencesKey("project_activity")

/** [ProjectActivity] kept in a preferences file of its own. */
@Singleton
class DataStoreProjectActivity
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : ProjectActivity {
        override fun observe(): Flow<Map<Long, Long>> =
            context.projectActivityPreferences.data.map { decodeProjectActivity(it[PROJECT_ACTIVITY]) }

        override suspend fun touch(
            projectLocalId: Long,
            at: Long,
        ) {
            val existing = observe().first()
            val next = withProjectActivity(existing, projectLocalId, at)
            if (next == existing) return
            context.projectActivityPreferences.edit { it[PROJECT_ACTIVITY] = encodeProjectActivity(next) }
        }

        override suspend fun clear() {
            context.projectActivityPreferences.edit { it.remove(PROJECT_ACTIVITY) }
        }
    }
