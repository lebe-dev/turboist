package ru.tinyops.turboist.nativeapp.auth

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.serialization.json.JsonObject
import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.LoginOutcome
import ru.tinyops.turboist.core.network.dto.PasskeyCeremonyDto
import ru.tinyops.turboist.core.network.dto.PasskeyConfigDto
import ru.tinyops.turboist.core.network.dto.SessionDto
import ru.tinyops.turboist.core.network.dto.UserDto

/**
 * A server that answers exactly what a test tells it to.
 *
 * The session rules worth testing are about what happens *around* the calls —
 * which failure keeps a token and which throws it away — so the calls
 * themselves are scripted rather than served over HTTP.
 */
internal class FakeAuthGateway : AuthGateway {
    var setup: SetupProbe = SetupProbe.DONE
    var reachable: Boolean = true

    /** Rotations handed out, oldest first. Each renewal takes the next one. */
    val rotations: ArrayDeque<Result<SessionDto>> = ArrayDeque()

    var signInResult: () -> LoginOutcome = { LoginOutcome.Session(sessionOf("access-in", "refresh-in")) }
    var otpResult: () -> SessionDto = { sessionOf("access-otp", "refresh-otp") }
    var createResult: () -> SessionDto = { sessionOf("access-new", "refresh-new") }

    /** What the server says about passkeys when the sign-in screen asks. */
    var passkeyOffer: PasskeyConfigDto = PasskeyConfigDto(enabled = true, available = true)

    /** The ceremony a passkey login is started with, or a failure to start one. */
    var passkeyCeremony: () -> PasskeyCeremonyDto = { passkeyCeremonyOf() }

    /** What the server answers a passkey assertion with. */
    var passkeyLoginResult: () -> SessionDto = { sessionOf("access-passkey", "refresh-passkey") }

    /** The credential last posted to the finish call, so a test can pin it travelled intact. */
    var assertedCredential: JsonObject? = null

    var passkeyCeremonyIds: MutableList<String> = mutableListOf()

    var presentedRefreshTokens: MutableList<String> = mutableListOf()
    var renewals: Int = 0
    var revocations: Int = 0

    /** How many of those revocations asked for every session rather than only this one. */
    var everySessionRevocations: Int = 0
    var revokeFails: Boolean = false

    override suspend fun ping() {
        if (!reachable) throw networkFailure()
    }

    override suspend fun probeSetup(): SetupProbe = setup

    override suspend fun createAccount(
        username: String,
        password: String,
    ): SessionDto = createResult()

    override suspend fun signIn(
        username: String,
        password: String,
    ): LoginOutcome = signInResult()

    override suspend fun verifyOtp(
        ticket: String,
        code: String,
    ): SessionDto = otpResult()

    override suspend fun passkeySupport(): PasskeyConfigDto {
        if (!reachable) throw networkFailure()
        return passkeyOffer
    }

    override suspend fun beginPasskeyLogin(): PasskeyCeremonyDto = passkeyCeremony()

    override suspend fun finishPasskeyLogin(
        ceremonyId: String,
        credential: JsonObject,
    ): SessionDto {
        passkeyCeremonyIds += ceremonyId
        assertedCredential = credential
        return passkeyLoginResult()
    }

    override suspend fun renew(refreshToken: String): SessionDto {
        renewals++
        presentedRefreshTokens += refreshToken
        val next = rotations.removeFirstOrNull() ?: throw networkFailure()
        return next.getOrThrow()
    }

    override suspend fun revokeSession() {
        revocations++
        if (revokeFails) throw networkFailure()
    }

    override suspend fun revokeEverySession() {
        revocations++
        everySessionRevocations++
        if (revokeFails) throw networkFailure()
    }
}

/** A ceremony in the shape the server sends it: the options wrapped for a browser. */
internal fun passkeyCeremonyOf(
    id: String = "ceremony-1",
    challenge: String = "q-_9AAECAwQFBgcICQoLDA0ODw",
): PasskeyCeremonyDto =
    TurboistJson.decodeFromString(
        PasskeyCeremonyDto.serializer(),
        """{"ceremonyId":"$id","options":{"publicKey":{"challenge":"$challenge","rpId":"todo.example.com"}}}""",
    )

internal fun sessionOf(
    access: String,
    refresh: String,
): SessionDto = SessionDto(access = access, refresh = refresh, user = UserDto(id = 1, username = "alice"))

internal fun networkFailure(): ApiException = ApiException.Network("the request did not reach the server")

internal fun rejection(): ApiException =
    ApiException.from(401, ApiErrorBody(code = ApiErrorCodes.AUTH_INVALID, message = "refresh token rejected"))

/** Remembers a token in memory, and counts what was done to it. */
internal class FakeRefreshTokenStore(
    private var token: String? = null,
) : RefreshTokenStore {
    /** Every token written, in order, so a test can pin when a rotation was stored. */
    val writes: MutableList<String> = mutableListOf()

    override suspend fun read(): String? = token

    override suspend fun write(token: String) {
        this.token = token
        writes += token
    }

    override suspend fun clear() {
        token = null
    }

    fun stored(): String? = token
}

internal class FakeServerAddressStore(
    private var address: String? = null,
) : ServerAddressStore {
    override suspend fun read(): String? = address

    override suspend fun write(url: String) {
        address = url
    }

    override suspend fun clear() {
        address = null
    }

    fun stored(): String? = address
}

/**
 * A network the test switches on by hand.
 *
 * The platform's callback is the only thing this stands in for: what matters to
 * the session rules is what happens *when* connectivity returns, not how the
 * app is told about it.
 */
internal class FakeNetworkAvailability : NetworkAvailability {
    private val events = MutableSharedFlow<Unit>(extraBufferCapacity = 8)

    override fun whenAvailable(): Flow<Unit> = events

    /** Announces a usable network, as the platform callback would. */
    suspend fun announce() {
        events.emit(Unit)
    }
}

/** A replica that only counts what was asked of it. */
internal class FakeLocalReplica(
    var unsent: Int = 0,
) : LocalReplica {
    var wipes: Int = 0

    override suspend fun unsentChangeCount(): Int = unsent

    override suspend fun wipe() {
        wipes++
        unsent = 0
    }
}
