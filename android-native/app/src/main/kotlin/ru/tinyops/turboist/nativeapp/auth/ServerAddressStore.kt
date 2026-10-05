package ru.tinyops.turboist.nativeapp.auth

import android.content.Context
import androidx.datastore.preferences.core.edit
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Where the address of the server this installation talks to is remembered.
 *
 * It survives logout. The product is self-hosted, so the address is something
 * the user configured rather than something they proved — forgetting it on
 * logout would make signing back in start with typing a URL again.
 */
interface ServerAddressStore {
    suspend fun read(): String?

    suspend fun write(url: String)

    suspend fun clear()
}

/** The [ServerAddressStore] backed by the session preferences file. */
@Singleton
class DataStoreServerAddressStore
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : ServerAddressStore {
        override suspend fun read(): String? =
            context.sessionPreferences.data.map { it[SessionKeys.SERVER_ADDRESS] }.first()

        override suspend fun write(url: String) {
            context.sessionPreferences.edit { it[SessionKeys.SERVER_ADDRESS] = url }
        }

        override suspend fun clear() {
            context.sessionPreferences.edit { it.remove(SessionKeys.SERVER_ADDRESS) }
        }
    }
