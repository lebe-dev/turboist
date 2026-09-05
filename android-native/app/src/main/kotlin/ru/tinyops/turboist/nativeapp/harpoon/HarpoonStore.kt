package ru.tinyops.turboist.nativeapp.harpoon

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The two things the user hops between, as this device holds them.
 *
 * The pair itself belongs to the account — it is kept with the user's own
 * preferences on the server and every change to it is sent there — but it is not
 * part of the data the replica copies down, so the device keeps its own copy of
 * it. That copy is what the jump control reads, and it is written the moment the
 * user hooks something on rather than when the queue drains, so the control is
 * right immediately and with no network at all.
 *
 * It is kept beside the replica rather than in it, for the same reason the
 * remembered projects and searches are: emptying the copied data, which a full
 * re-copy does, must not empty the pair the user is hopping between.
 */
interface HarpoonStore {
    /** The pair, oldest first, and again whenever it changes. */
    fun observe(): Flow<List<HarpoonEntry>>

    /** Replaces the pair. */
    suspend fun save(entries: List<HarpoonEntry>)

    /** Forgets it. */
    suspend fun clear()
}

/**
 * The key-value file the jump pair lives in.
 *
 * Its own file rather than a corner of another: a process may open a given
 * DataStore file only once, and nothing here has anything to do with capture,
 * searching or signing in.
 */
private val Context.harpoonPreferences: DataStore<Preferences> by preferencesDataStore(name = "harpoon")

private val HARPOONED = stringPreferencesKey("harpooned")

/**
 * [HarpoonStore] kept in that file.
 *
 * Stored as `kind:id` pairs separated by commas. An entry that cannot be read
 * back is dropped rather than failing the read: the pair is a navigation
 * convenience, and one the device cannot make sense of has simply not been
 * hooked on.
 */
@Singleton
class DataStoreHarpoonStore
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : HarpoonStore {
        override fun observe(): Flow<List<HarpoonEntry>> =
            context.harpoonPreferences.data.map { preferences ->
                preferences[HARPOONED].orEmpty()
                    .split(SEPARATOR)
                    .mapNotNull(::parse)
            }

        override suspend fun save(entries: List<HarpoonEntry>) {
            val stored = entries.joinToString(SEPARATOR) { it.target.wire + FIELD + it.localId }
            context.harpoonPreferences.edit { it[HARPOONED] = stored }
        }

        override suspend fun clear() {
            context.harpoonPreferences.edit { it.remove(HARPOONED) }
        }

        private fun parse(stored: String): HarpoonEntry? {
            val kind = stored.substringBefore(FIELD, missingDelimiterValue = "")
            val localId = stored.substringAfter(FIELD, missingDelimiterValue = "").toLongOrNull() ?: return null
            val target = HarpoonTarget.entries.firstOrNull { it.wire == kind } ?: return null
            return HarpoonEntry(target, localId)
        }

        private companion object {
            const val SEPARATOR = ","
            const val FIELD = ":"
        }
    }
