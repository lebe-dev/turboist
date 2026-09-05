package ru.tinyops.turboist.nativeapp.auth

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Log
import androidx.datastore.preferences.core.edit
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The refresh token, sealed with a key the app can use but never read.
 *
 * The key is generated inside the platform keystore and stays there: this code
 * gets a handle to it, not the bytes, so the ciphertext sitting in app storage
 * is worthless on any other device and worthless on this one to anything that
 * cannot act as this app. That is the whole reason the token is not simply
 * written into the same preferences file as the server address.
 *
 * Sealing is AES-GCM, which authenticates as well as encrypts: a tampered blob
 * fails to open rather than decrypting into a token the server would then treat
 * as stolen and use to kill the session.
 *
 * Failing to open a stored blob is treated as having no token at all. The key
 * can genuinely disappear — a restore onto another device, or the platform
 * invalidating it — and the honest response is to ask the user to sign in again
 * rather than to crash on every launch from then on.
 */
@Singleton
class KeystoreRefreshTokenStore
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : RefreshTokenStore {
        override suspend fun read(): String? {
            val sealed = context.sessionPreferences.data.map { it[SessionKeys.REFRESH_TOKEN] }.first() ?: return null
            return try {
                open(sealed)
            } catch (e: GeneralSecurityException) {
                Log.w(TAG, "The stored sign-in token could not be opened; asking for a fresh sign-in", e)
                clear()
                null
            } catch (e: IllegalArgumentException) {
                Log.w(TAG, "The stored sign-in token is not readable; asking for a fresh sign-in", e)
                clear()
                null
            }
        }

        override suspend fun write(token: String) {
            val sealed = seal(token)
            context.sessionPreferences.edit { it[SessionKeys.REFRESH_TOKEN] = sealed }
        }

        override suspend fun clear() {
            context.sessionPreferences.edit { it.remove(SessionKeys.REFRESH_TOKEN) }
        }

        /** Encrypts and packs the initialisation vector in front of the ciphertext. */
        private fun seal(token: String): String {
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.ENCRYPT_MODE, sealingKey())
            val ciphertext = cipher.doFinal(token.toByteArray(Charsets.UTF_8))
            val packed = cipher.iv + ciphertext
            return Base64.getEncoder().encodeToString(packed)
        }

        private fun open(sealed: String): String {
            val packed = Base64.getDecoder().decode(sealed)
            require(packed.size > IV_BYTES) { "the sealed token is too short to contain one" }
            val cipher = Cipher.getInstance(TRANSFORMATION)
            val spec = GCMParameterSpec(TAG_BITS, packed, 0, IV_BYTES)
            cipher.init(Cipher.DECRYPT_MODE, sealingKey(), spec)
            val plaintext = cipher.doFinal(packed, IV_BYTES, packed.size - IV_BYTES)
            return String(plaintext, Charsets.UTF_8)
        }

        /**
         * The device-bound key, created on first use.
         *
         * It is deliberately not bound to the screen lock: the app has to be able
         * to refresh its session in the background, and a key that only opens
         * while the device is unlocked would turn every background sync into a
         * sign-out.
         */
        private fun sealingKey(): SecretKey {
            val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
            (keyStore.getEntry(ALIAS, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
            val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
            generator.init(
                KeyGenParameterSpec
                    .Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .setKeySize(KEY_BITS)
                    .build(),
            )
            return generator.generateKey()
        }

        private companion object {
            const val TAG = "TurboistSession"
            const val KEYSTORE = "AndroidKeyStore"
            const val ALIAS = "turboist.session.refresh"
            const val TRANSFORMATION = "AES/GCM/NoPadding"
            const val KEY_BITS = 256
            const val TAG_BITS = 128

            /** What the platform generates for GCM, and what the packed blob starts with. */
            const val IV_BYTES = 12
        }
    }
