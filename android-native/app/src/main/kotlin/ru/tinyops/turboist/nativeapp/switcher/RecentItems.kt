package ru.tinyops.turboist.nativeapp.switcher

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

/** How many opened tasks and projects are remembered. */
const val RECENT_ITEMS_MEMORY: Int = 30

/** How many of them the quick switcher shows before anything is typed. */
const val RECENT_ITEMS_SHOWN: Int = 8

/** The two kinds of thing the quick switcher finds and remembers. */
enum class SwitcherKind(
    internal val code: String,
) {
    TASK("t"),
    PROJECT("p"),
}

/** A remembered task or project, by the id this device holds its row under. */
data class RecentRef(
    val kind: SwitcherKind,
    val localId: Long,
)

/**
 * Puts a task or project at the front of the remembered order.
 *
 * Most-recent-first with no duplicates, capped. Pure, because the order, the
 * de-duplication and the cap are what have to stay true.
 */
fun withRecentItem(
    existing: List<RecentRef>,
    ref: RecentRef,
    limit: Int = RECENT_ITEMS_MEMORY,
): List<RecentRef> {
    if (limit <= 0) return emptyList()
    return (listOf(ref) + existing.filterNot { it == ref }).take(limit)
}

/** The stored form: `t:12,p:3`. An entry that cannot be read is dropped. */
internal fun encodeRecentItems(refs: List<RecentRef>): String =
    refs.joinToString(",") { "${it.kind.code}:${it.localId}" }

internal fun decodeRecentItems(stored: String?): List<RecentRef> =
    stored.orEmpty().split(",").mapNotNull { entry ->
        val kind = SwitcherKind.entries.firstOrNull { entry.startsWith(it.code + ":") } ?: return@mapNotNull null
        entry.substringAfter(":").toLongOrNull()?.let { RecentRef(kind, it) }
    }

/**
 * The tasks and projects this device opened lately, for the quick switcher.
 *
 * Device-local, like the project picker's order: a navigation habit, never
 * synced. Rows are remembered by local id, so one created offline is remembered
 * like any other; the titles are always read live from the replica, so nothing
 * here can go stale.
 */
interface RecentItems {
    /** The remembered order, most recent first, and again whenever it changes. */
    fun observe(): Flow<List<RecentRef>>

    /** Records that a task or project was just opened. */
    suspend fun visit(ref: RecentRef)

    /** Forgets all of them. */
    suspend fun clear()
}

/** Its own file: a process may open a given DataStore file only once. */
private val Context.switcherPreferences: DataStore<Preferences> by preferencesDataStore(name = "quick_switcher")

private val RECENT_ITEMS = stringPreferencesKey("recent_items")

/** [RecentItems] kept in the quick switcher's preferences file. */
@Singleton
class DataStoreRecentItems
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : RecentItems {
        override fun observe(): Flow<List<RecentRef>> =
            context.switcherPreferences.data.map { decodeRecentItems(it[RECENT_ITEMS]) }

        override suspend fun visit(ref: RecentRef) {
            val existing = observe().first()
            val next = withRecentItem(existing, ref)
            if (next == existing) return
            context.switcherPreferences.edit { it[RECENT_ITEMS] = encodeRecentItems(next) }
        }

        override suspend fun clear() {
            context.switcherPreferences.edit { it.remove(RECENT_ITEMS) }
        }
    }
