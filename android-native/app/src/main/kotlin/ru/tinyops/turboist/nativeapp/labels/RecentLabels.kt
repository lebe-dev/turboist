package ru.tinyops.turboist.nativeapp.labels

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
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.nativeapp.quickadd.pickRecent
import ru.tinyops.turboist.nativeapp.quickadd.withRecentProject
import javax.inject.Inject
import javax.inject.Singleton

/** How many labels are remembered. Deeper than any row shows, for the reason [pickRecent] gives. */
const val RECENT_LABELS_MEMORY: Int = 12

/** How many of them a picker leads with. */
const val RECENT_LABELS_SHOWN: Int = 5

/** Puts a label at the front of the remembered order: most recent first, no duplicates, capped. */
fun withRecentLabel(
    existing: List<Long>,
    labelLocalId: Long,
    limit: Int = RECENT_LABELS_MEMORY,
): List<Long> = withRecentProject(existing, labelLocalId, limit)

/**
 * The recent slice of [candidates], most recent first, or nothing when the whole
 * list already fits in the row. The caller passes the labels it is about to
 * offer, so a label hidden by the search box never comes back through the row.
 */
fun pickRecentLabels(
    order: List<Long>,
    candidates: List<Label>,
): List<Label> = pickRecent(order, candidates, Label::localId, RECENT_LABELS_SHOWN)

/**
 * The labels this device has applied to tasks lately.
 *
 * Applying a label is the only thing recorded; taking one off is not, because
 * it says nothing about which labels the user reaches for. Device-local for the
 * same reason the recent projects are: it is a habit of the hand holding this
 * phone, not a record the workspace owns, so it is never synced.
 */
interface RecentLabels {
    /** The remembered order, most recent first, and again whenever it changes. */
    fun observe(): Flow<List<Long>>

    /** Records that a label was just applied to a task. */
    suspend fun remember(labelLocalId: Long)

    /** Forgets all of them. */
    suspend fun clear()

    /** Remembers nothing — for screens and checks that have no use for the row. */
    object None : RecentLabels {
        override fun observe(): Flow<List<Long>> = flowOf(emptyList())

        override suspend fun remember(labelLocalId: Long) = Unit

        override suspend fun clear() = Unit
    }
}

/** Its own file: a process may open a given DataStore file only once. */
private val Context.labelPreferences: DataStore<Preferences> by preferencesDataStore(name = "labels")

private val RECENT_LABEL_IDS = stringPreferencesKey("recent_labels")

/**
 * [RecentLabels] kept in a small preferences file, as ids separated by commas.
 * An entry that cannot be read as a number is dropped rather than failing the read.
 */
@Singleton
class DataStoreRecentLabels
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : RecentLabels {
        override fun observe(): Flow<List<Long>> =
            context.labelPreferences.data.map { preferences ->
                preferences[RECENT_LABEL_IDS].orEmpty().split(SEPARATOR).mapNotNull(String::toLongOrNull)
            }

        override suspend fun remember(labelLocalId: Long) {
            val existing = observe().first()
            val next = withRecentLabel(existing, labelLocalId)
            if (next == existing) return
            context.labelPreferences.edit { it[RECENT_LABEL_IDS] = next.joinToString(SEPARATOR) }
        }

        override suspend fun clear() {
            context.labelPreferences.edit { it.remove(RECENT_LABEL_IDS) }
        }

        private companion object {
            const val SEPARATOR = ","
        }
    }
