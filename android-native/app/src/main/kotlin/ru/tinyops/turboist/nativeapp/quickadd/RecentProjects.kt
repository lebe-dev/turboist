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
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

/** How many projects are remembered. Deeper than any row shows, on purpose — see [pickRecent]. */
const val RECENT_PROJECTS_MEMORY: Int = 12

/** How many of them a picker leads with. */
const val RECENT_PROJECTS_SHOWN: Int = 3

/**
 * Puts a project at the front of the remembered order.
 *
 * Most-recent-first with no duplicates: filing a second task into the same
 * project moves it up rather than adding a second entry. Written as a pure
 * function because the order, the de-duplication and the cap are what have to
 * stay true, and that is worth checking without a file on disk in the way.
 */
fun withRecentProject(
    existing: List<Long>,
    projectLocalId: Long,
    limit: Int = RECENT_PROJECTS_MEMORY,
): List<Long> {
    if (limit <= 0) return emptyList()
    return (listOf(projectLocalId) + existing.filterNot { it == projectLocalId }).take(limit)
}

/**
 * The recent slice of [candidates], most recent first.
 *
 * The caller passes its already-filtered list, so a project the picker is hiding
 * — because the search box narrowed it, or because it is finished — never
 * reappears in the recent row either. That filtering is also why more ids are
 * remembered than are ever shown: a shallow memory would come back empty as soon
 * as the user typed anything.
 *
 * Nothing is returned when the whole list already fits in the row. A "recent"
 * group that repeats the entire picker is noise, and worse, it offers the same
 * project twice.
 */
fun <T> pickRecent(
    order: List<Long>,
    candidates: List<T>,
    localIdOf: (T) -> Long,
    limit: Int = RECENT_PROJECTS_SHOWN,
): List<T> {
    if (limit <= 0 || candidates.size <= limit) return emptyList()
    val byId = candidates.associateBy(localIdOf)
    val picked = ArrayList<T>(limit)
    for (localId in order) {
        val hit = byId[localId] ?: continue
        picked += hit
        if (picked.size == limit) break
    }
    return picked
}

/**
 * The projects this device has filed work into lately.
 *
 * Device-local and nothing else. Which project the phone in your pocket reaches
 * for is a navigation habit, not a record the workspace owns: it is never
 * synced, never sent anywhere, and no server field exists for it. Sharing it
 * between devices would also make it worse — the row is short precisely so the
 * places *this* device files things are one tap away.
 *
 * Projects are remembered by the id this device holds them under, so a project
 * created offline is remembered like any other and keeps its place when the
 * server's id is written onto the row later.
 */
interface RecentProjects {
    /** The remembered order, most recent first, and again whenever it changes. */
    fun observe(): Flow<List<Long>>

    /** Records that work was just filed into a project. */
    suspend fun remember(projectLocalId: Long)

    /** Forgets all of them. */
    suspend fun clear()
}

/**
 * The key-value file the capture surface keeps beside itself.
 *
 * Its own file rather than a corner of another: a process may open a given
 * DataStore file only once, and nothing here has anything to do with signing in
 * or with searching.
 */
private val Context.capturePreferences: DataStore<Preferences> by preferencesDataStore(name = "capture")

private val RECENT_PROJECT_IDS = stringPreferencesKey("recent_projects")

/**
 * [RecentProjects] kept in the capture preferences file.
 *
 * Stored as ids separated by commas. An entry that cannot be read as a number is
 * dropped rather than failing the read: a stored habit is not worth a crash, and
 * an unreadable one simply has not been visited.
 */
@Singleton
class DataStoreRecentProjects
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : RecentProjects {
        override fun observe(): Flow<List<Long>> =
            context.capturePreferences.data.map { preferences ->
                preferences[RECENT_PROJECT_IDS].orEmpty().split(SEPARATOR).mapNotNull(String::toLongOrNull)
            }

        override suspend fun remember(projectLocalId: Long) {
            val existing = observe().first()
            val next = withRecentProject(existing, projectLocalId)
            if (next == existing) return
            context.capturePreferences.edit { it[RECENT_PROJECT_IDS] = next.joinToString(SEPARATOR) }
        }

        override suspend fun clear() {
            context.capturePreferences.edit { it.remove(RECENT_PROJECT_IDS) }
        }

        private companion object {
            const val SEPARATOR = ","
        }
    }
