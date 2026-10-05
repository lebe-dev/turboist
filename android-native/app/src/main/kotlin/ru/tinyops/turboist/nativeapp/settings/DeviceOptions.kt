package ru.tinyops.turboist.nativeapp.settings

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

/** Which colours the app draws itself in, regardless of what any other device does. */
enum class ThemeChoice {
    /** Follow whatever the phone is set to. The default, and what most people want. */
    SYSTEM,
    LIGHT,
    DARK,
    ;

    companion object {
        /**
         * Reads a stored choice back.
         *
         * An unreadable value falls back to following the phone rather than
         * failing: a preferences file written by a newer build must not be able
         * to stop this one from drawing.
         */
        fun ofStored(value: String?): ThemeChoice = entries.firstOrNull { it.name == value } ?: SYSTEM
    }
}

/**
 * The choices that belong to this phone rather than to the account.
 *
 * They are deliberately not preferences on the server and never travel with the
 * user: which colours a screen is drawn in and whether a radio may spend the
 * user's data allowance are facts about a device, and copying them onto a second
 * one would be wrong on both counts — a tablet on wi-fi and a phone on a metered
 * plan want opposite answers to the same question.
 *
 * @property syncOnMetered whether the background catch-up may run on a metered
 *   connection. Turning it off never blocks the user: a refresh they asked for
 *   is still made, because that is a person waiting rather than a timer firing.
 */
data class DeviceOptions(
    val theme: ThemeChoice = ThemeChoice.SYSTEM,
    val syncOnMetered: Boolean = true,
)

/** Where the device's own choices are kept. */
interface DeviceOptionsStore {
    /** The current choices, and again whenever one is changed. */
    fun observe(): Flow<DeviceOptions>

    /** The current choices, once. For the places that need an answer before drawing anything. */
    suspend fun read(): DeviceOptions

    suspend fun setTheme(theme: ThemeChoice)

    suspend fun setSyncOnMetered(allowed: Boolean)
}

/**
 * The key-value file the device's own choices live in.
 *
 * Its own file rather than a corner of the session's: a process may open a given
 * DataStore file only once, and nothing here has anything to do with signing in
 * — these choices outlive every session the device ever has.
 */
private val Context.deviceOptions: DataStore<Preferences> by preferencesDataStore(name = "device")

private val THEME = stringPreferencesKey("theme")
private val SYNC_ON_METERED = booleanPreferencesKey("sync_on_metered")

/** [DeviceOptionsStore] kept in the device's own preferences file. */
@Singleton
class DataStoreDeviceOptions
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : DeviceOptionsStore {
        override fun observe(): Flow<DeviceOptions> = context.deviceOptions.data.map(::optionsOf)

        override suspend fun read(): DeviceOptions = observe().first()

        override suspend fun setTheme(theme: ThemeChoice) {
            context.deviceOptions.edit { it[THEME] = theme.name }
        }

        override suspend fun setSyncOnMetered(allowed: Boolean) {
            context.deviceOptions.edit { it[SYNC_ON_METERED] = allowed }
        }

        private fun optionsOf(stored: Preferences): DeviceOptions =
            DeviceOptions(
                theme = ThemeChoice.ofStored(stored[THEME]),
                // Absent means the user has never said, and a device that has
                // never been asked syncs: an app that quietly stopped catching up
                // until a switch was found would look broken rather than frugal.
                syncOnMetered = stored[SYNC_ON_METERED] ?: true,
            )
    }
