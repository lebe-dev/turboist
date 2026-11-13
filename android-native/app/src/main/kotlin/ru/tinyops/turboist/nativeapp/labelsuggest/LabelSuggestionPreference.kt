package ru.tinyops.turboist.nativeapp.labelsuggest

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

private val Context.labelSuggestionOptions: DataStore<Preferences> by preferencesDataStore(name = "label_suggestions")

private val ENABLED = booleanPreferencesKey("enabled")

/**
 * Whether this phone suggests labels.
 *
 * A device choice, off until the user turns it on: it costs a large download
 * and a few hundred megabytes of memory while it runs, which is not something
 * to spend on someone's behalf. Kept apart from the account — a tablet might
 * have the room for it and a phone not.
 */
@Singleton
class LabelSuggestionPreference
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) {
        val enabled: Flow<Boolean> = context.labelSuggestionOptions.data.map { it[ENABLED] ?: false }

        suspend fun setEnabled(enabled: Boolean) {
            context.labelSuggestionOptions.edit { it[ENABLED] = enabled }
        }
    }
