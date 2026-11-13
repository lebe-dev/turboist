package ru.tinyops.turboist.nativeapp.labelsuggest

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

// The file and the labels key keep the names they had before project
// suggestions existed, so a phone that already had label suggestions on keeps them.
private val Context.suggestionOptions: DataStore<Preferences> by preferencesDataStore(name = "label_suggestions")

private val LABELS = booleanPreferencesKey("enabled")
private val PROJECTS = booleanPreferencesKey("projects_enabled")

/**
 * Whether this phone suggests labels, and whether it suggests projects.
 *
 * Two device choices, each off until the user turns it on. Both need the same
 * model, a large download that holds a few hundred megabytes of memory while it
 * runs — not something to spend on someone's behalf. Kept apart from the
 * account: a tablet might have the room for it and a phone not.
 */
@Singleton
class SuggestionPreference
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) {
        /** Both switches as one value. */
        val switches: Flow<SuggestionSwitches> =
            context.suggestionOptions.data
                .map { SuggestionSwitches(labels = it[LABELS] ?: false, projects = it[PROJECTS] ?: false) }
                .distinctUntilChanged()

        suspend fun setLabelsEnabled(enabled: Boolean) {
            context.suggestionOptions.edit { it[LABELS] = enabled }
        }

        suspend fun setProjectsEnabled(enabled: Boolean) {
            context.suggestionOptions.edit { it[PROJECTS] = enabled }
        }

        /** Turns both off at once, as deleting the model does. */
        suspend fun disableAll() {
            context.suggestionOptions.edit {
                it[LABELS] = false
                it[PROJECTS] = false
            }
        }
    }

/** Which kinds of suggestion are switched on. */
data class SuggestionSwitches(
    val labels: Boolean = false,
    val projects: Boolean = false,
) {
    val any: Boolean get() = labels || projects
}
