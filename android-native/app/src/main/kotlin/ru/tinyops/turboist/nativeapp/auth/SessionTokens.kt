package ru.tinyops.turboist.nativeapp.auth

import ru.tinyops.turboist.core.network.auth.AccessTokenSource
import java.util.concurrent.atomic.AtomicReference
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The access token, and the only copy of it.
 *
 * It is held in memory and nowhere else. A 15-minute token is cheap to replace
 * from the refresh token, so writing it down would add a second credential at
 * rest to buy nothing. Killing the process is therefore enough to forget it,
 * and the next launch mints a new one.
 *
 * Reads come from arbitrary HTTP threads and writes from the session layer, so
 * the value is atomic.
 */
@Singleton
class SessionTokens
    @Inject
    constructor() : AccessTokenSource {
        private val current = AtomicReference<String?>(null)

        override fun accessToken(): String? = current.get()

        fun set(token: String?) {
            current.set(token)
        }
    }
