package ru.tinyops.turboist.nativeapp.calendar

import android.content.Context
import android.util.Log
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.serialization.SerializationException
import ru.tinyops.turboist.core.network.TurboistJson
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The stored calendar copy, in a small key-value file of its own.
 *
 * A file of its own, and not a row in the replica database, is the whole point:
 * see [CalendarCache]. It holds one value — the last answer, as JSON — because
 * there is only ever one span worth keeping, and replacing it wholesale is
 * simpler to reason about than reconciling overlapping spans.
 *
 * The file is private to the app. It is emptied when the user signs out, along
 * with everything else on the device that belongs to them.
 */
private val Context.calendarPreferences: DataStore<Preferences> by preferencesDataStore(name = "calendar")

private val CACHED_ANSWER = stringPreferencesKey("cached_answer")

private const val TAG = "CalendarCache"

@Singleton
class DataStoreCalendarCache
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : CalendarCache {
        /**
         * A copy this build cannot read is treated as no copy at all.
         *
         * The alternative — failing — would take a screen down over an old file,
         * and there is nothing here worth that: the copy exists only to fill in
         * for a server that is briefly out of reach, and the next successful
         * request replaces it.
         */
        override suspend fun read(): CalendarCacheEntry? {
            val stored = context.calendarPreferences.data.map { it[CACHED_ANSWER] }.first() ?: return null
            return try {
                TurboistJson.decodeFromString(CalendarCacheEntry.serializer(), stored)
            } catch (e: SerializationException) {
                Log.i(TAG, "The stored calendar copy could not be read and was ignored", e)
                null
            }
        }

        override suspend fun write(entry: CalendarCacheEntry) {
            val encoded = TurboistJson.encodeToString(CalendarCacheEntry.serializer(), entry)
            context.calendarPreferences.edit { it[CACHED_ANSWER] = encoded }
        }

        override suspend fun clear() {
            context.calendarPreferences.edit { it.remove(CACHED_ANSWER) }
        }
    }
