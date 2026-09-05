package ru.tinyops.turboist.nativeapp.auth

/**
 * Where the one persisted secret of this app lives.
 *
 * The rotating refresh token is the only credential written to disk: the access
 * token is short-lived and stays in memory, and there is no password to keep.
 * That makes this the whole at-rest attack surface of the session, which is why
 * it is an interface with exactly three operations — anything more would be
 * another way for the token to leak.
 *
 * Every implementation must seal the value: the token is a bearer credential,
 * and a copy of it read off a rooted device is a session.
 */
interface RefreshTokenStore {
    /** The stored token, or `null` when this device has never signed in. */
    suspend fun read(): String?

    /**
     * Replaces the stored token.
     *
     * Called on every rotation, and it has to complete before the new session is
     * used: the server kills a session that sees its previous token again, so a
     * rotation that was used but not stored locks the device out at next launch.
     */
    suspend fun write(token: String)

    /** Forgets the token, which is what signing out means on this device. */
    suspend fun clear()
}
