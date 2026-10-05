package ru.tinyops.turboist.nativeapp.account

import ru.tinyops.turboist.core.network.ApiErrorBody
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.network.dto.ActiveSessionDto
import ru.tinyops.turboist.core.network.dto.ApiTokenDto
import ru.tinyops.turboist.core.network.dto.CreatedApiTokenDto
import ru.tinyops.turboist.core.network.dto.TotpEnrolmentDto

internal fun offline(): ApiException = ApiException.Network("the request did not reach the server")

internal fun refusal(
    status: Int = 409,
    code: String = ApiErrorCodes.CONFLICT,
): ApiException = ApiException.from(status, ApiErrorBody(code = code, message = "refused"))

/** What a server answers when the route was never registered on this deployment. */
internal fun absentRoute(): ApiException =
    ApiException.from(404, ApiErrorBody(code = ApiErrorCodes.NOT_FOUND, message = "not found"))

internal fun sessionOf(
    id: Long,
    kind: String = "web",
    current: Boolean = false,
): ActiveSessionDto =
    ActiveSessionDto(
        id = id,
        clientKind = kind,
        displayName = "Device $id",
        createdAt = "2026-08-18T10:00:00.000Z",
        lastUsedAt = "2026-08-20T09:12:00.000Z",
        isCurrent = current,
    )

/** A server holding a list of sessions, plus the local half of signing out. */
internal class FakeSessionControl : SessionControl {
    val stored = mutableListOf<ActiveSessionDto>()
    val revoked = mutableListOf<Long>()
    var listFailure: Throwable? = null
    var revokeFailure: Throwable? = null
    var othersSignedOut = 0
    var signedOutEverywhere = 0
    var unsent = 0

    override suspend fun list(): List<ActiveSessionDto> {
        listFailure?.let { throw it }
        return stored.toList()
    }

    override suspend fun revoke(id: Long) {
        revokeFailure?.let { throw it }
        revoked += id
        stored.removeAll { it.id == id }
    }

    override suspend fun signOutOthers() {
        othersSignedOut++
        stored.removeAll { !it.isCurrent }
    }

    override suspend fun unsentChangeCount(): Int = unsent

    override suspend fun signOutEverywhere() {
        signedOutEverywhere++
        stored.clear()
    }
}

/** A server that mints tokens and hands the plaintext over exactly once. */
internal class FakeApiTokenControl : ApiTokenControl {
    val stored = mutableListOf<ApiTokenDto>()
    val created = mutableListOf<Pair<String, List<String>>>()
    val deleted = mutableListOf<Long>()
    var listFailure: Throwable? = null
    var createFailure: Throwable? = null
    var deleteFailure: Throwable? = null

    override suspend fun list(): List<ApiTokenDto> {
        listFailure?.let { throw it }
        return stored.toList()
    }

    override suspend fun create(
        name: String,
        scopes: List<String>,
    ): CreatedApiTokenDto {
        createFailure?.let { throw it }
        created += name to scopes
        val minted =
            CreatedApiTokenDto(
                id = (stored.maxOfOrNull { it.id } ?: 0) + 1,
                name = name,
                scopes = scopes,
                token = "plaintext-$name",
                createdAt = "2026-08-20T10:00:00.000Z",
            )
        stored += ApiTokenDto(id = minted.id, name = minted.name, scopes = minted.scopes, createdAt = minted.createdAt)
        return minted
    }

    override suspend fun delete(id: Long) {
        deleteFailure?.let { throw it }
        deleted += id
        stored.removeAll { it.id == id }
    }
}

/** A server that can be asked to turn a second factor on and off. */
internal class FakeTwoFactorControl : TwoFactorControl {
    var enabled = false
    var statusFailure: Throwable? = null
    var beginFailure: Throwable? = null
    var confirmFailure: Throwable? = null
    var disableFailure: Throwable? = null
    var beginCalls = 0
    val confirmedCodes = mutableListOf<String>()
    val disableCodes = mutableListOf<String>()
    var recoveryCodes = listOf("AAAA1111", "BBBB2222")

    override suspend fun isEnabled(): Boolean {
        statusFailure?.let { throw it }
        return enabled
    }

    override suspend fun begin(): TotpEnrolmentDto {
        beginFailure?.let { throw it }
        beginCalls++
        return TotpEnrolmentDto(
            secret = "JBSWY3DPEHPK3PXP",
            otpauthUrl = "otpauth://totp/Turboist:alice?secret=JBSWY3DPEHPK3PXP",
        )
    }

    override suspend fun confirm(code: String): List<String> {
        confirmFailure?.let { throw it }
        confirmedCodes += code
        enabled = true
        return recoveryCodes
    }

    override suspend fun disable(code: String) {
        disableFailure?.let { throw it }
        disableCodes += code
        enabled = false
    }
}
