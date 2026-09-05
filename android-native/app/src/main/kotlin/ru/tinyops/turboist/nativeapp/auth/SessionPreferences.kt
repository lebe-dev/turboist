package ru.tinyops.turboist.nativeapp.auth

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore

/**
 * The one small key-value file the session layer keeps.
 *
 * It holds two unrelated things — the address of the server and the sealed
 * refresh token — and they share a file on purpose: a process may open a given
 * DataStore file exactly once, so a second store declared elsewhere over the
 * same name would crash at the first read. One declaration, here, is what makes
 * that impossible.
 *
 * Nothing in this file is readable by another app: it lives in private storage,
 * and the one secret in it is sealed with a key that never leaves the device's
 * keystore.
 */
internal val Context.sessionPreferences: DataStore<Preferences> by preferencesDataStore(name = "session")

internal object SessionKeys {
    /** The address the user connected to. Configuration, not a credential: logout keeps it. */
    val SERVER_ADDRESS = stringPreferencesKey("server_address")

    /** The rotating refresh token, sealed. Never written in the clear. */
    val REFRESH_TOKEN = stringPreferencesKey("refresh_token")
}
