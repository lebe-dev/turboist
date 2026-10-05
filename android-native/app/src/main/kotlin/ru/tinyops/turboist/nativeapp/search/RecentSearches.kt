package ru.tinyops.turboist.nativeapp.search

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

/** How many past queries are kept. A short list the user can read at a glance. */
const val RECENT_SEARCH_LIMIT: Int = 8

/**
 * Reduces what someone typed to the form a past search is remembered in.
 *
 * Whitespace is collapsed and the ends are trimmed, so `renew   passport ` and
 * `renew passport` are one entry rather than two that look identical in a list.
 * It also guarantees the value holds no line break, which is what lets the list
 * be stored as lines.
 */
fun normalisedQuery(typed: String): String = typed.trim().replace(WHITESPACE, " ")

private val WHITESPACE = Regex("\\s+")

/**
 * Puts [query] at the front of the recent list.
 *
 * Most-recent-first with no duplicates: searching for the same thing twice moves
 * it up rather than adding a second row. Written as a pure function because the
 * rule — the order, the de-duplication, the cap — is what has to stay true, and
 * that is worth checking without a file on disk in the way.
 */
fun withRecentSearch(
    existing: List<String>,
    query: String,
    limit: Int = RECENT_SEARCH_LIMIT,
): List<String> {
    val normalised = normalisedQuery(query)
    if (normalised.isEmpty()) return existing
    return (listOf(normalised) + existing.filterNot { it.equals(normalised, ignoreCase = true) }).take(limit)
}

/**
 * The searches this device has run before.
 *
 * Device-local and nothing else: a past query is a fact about how this phone was
 * used, not a record the workspace owns, so it is never synced and never sent
 * anywhere. Sharing them between devices would also make them useless — the list
 * is short precisely so the last few things *you* looked for are one tap away.
 */
interface RecentSearches {
    /** The list, most recent first, and again whenever it changes. */
    fun observe(): Flow<List<String>>

    /** Records a query the user actually ran. */
    suspend fun remember(query: String)

    /** Forgets all of them. */
    suspend fun clear()
}

/**
 * The key-value file the search screen keeps beside itself.
 *
 * Its own file rather than a corner of the session's: a process may open a given
 * DataStore file only once, and nothing here has anything to do with signing in.
 */
private val Context.searchPreferences: DataStore<Preferences> by preferencesDataStore(name = "search")

private val RECENT_QUERIES = stringPreferencesKey("recent_queries")

/**
 * [RecentSearches] kept in the search preferences file.
 *
 * The list is stored as lines. A stored query can hold no line break by
 * construction — [normalisedQuery] collapses every run of whitespace to a single
 * space before anything is written — so the encoding is total and needs no
 * escaping.
 */
@Singleton
class DataStoreRecentSearches
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : RecentSearches {
        override fun observe(): Flow<List<String>> =
            context.searchPreferences.data.map { preferences ->
                preferences[RECENT_QUERIES].orEmpty().lines().filter { it.isNotBlank() }
            }

        override suspend fun remember(query: String) {
            val existing = observe().first()
            val next = withRecentSearch(existing, query)
            if (next == existing) return
            context.searchPreferences.edit { it[RECENT_QUERIES] = next.joinToString("\n") }
        }

        override suspend fun clear() {
            context.searchPreferences.edit { it.remove(RECENT_QUERIES) }
        }
    }
