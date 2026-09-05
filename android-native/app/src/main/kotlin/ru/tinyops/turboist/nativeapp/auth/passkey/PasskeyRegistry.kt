package ru.tinyops.turboist.nativeapp.auth.passkey

import ru.tinyops.turboist.core.network.api.PasskeyApi
import ru.tinyops.turboist.core.network.dto.PasskeyDto
import ru.tinyops.turboist.core.network.dto.PasskeyRegisterFinishRequest
import ru.tinyops.turboist.core.network.dto.PasskeyRenameRequest
import ru.tinyops.turboist.core.network.dto.optionsJson
import ru.tinyops.turboist.core.network.dto.passkeyCredentialOf
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The account's registered passkeys, and the enrolment ceremony that adds one.
 *
 * Deliberately online-only, and deliberately not part of the replica. A list of
 * credentials is an administrative surface with no offline use — a passkey
 * cannot be enrolled or revoked without the server agreeing — and caching it
 * would leave a stale answer to the one question where staleness is misleading:
 * which devices can sign in to this account.
 */
@Singleton
class PasskeyRegistry
    @Inject
    constructor(
        private val api: PasskeyApi,
    ) {
        suspend fun list(): List<PasskeyDto> = api.list()

        /**
         * Enrols this device.
         *
         * The server decides what the ceremony asks for — including which
         * credentials the device already has, so the platform offers to replace
         * one instead of stacking a duplicate — and verifies the attestation that
         * comes back. This only carries the JSON between the two.
         *
         * @param name the user's label for the device, or `null` to let the server
         *   name it rather than sending a placeholder invented here.
         */
        suspend fun register(
            authenticator: PasskeyAuthenticator,
            name: String?,
        ): PasskeyDto {
            val ceremony = api.registerBegin()
            val created = authenticator.registration(ceremony.optionsJson())
            return api.registerFinish(
                PasskeyRegisterFinishRequest(
                    ceremonyId = ceremony.ceremonyId,
                    credential = passkeyCredentialOf(created),
                    name = name?.trim()?.takeIf { it.isNotEmpty() },
                ),
            )
        }

        suspend fun rename(
            id: Long,
            name: String,
        ): PasskeyDto = api.rename(id, PasskeyRenameRequest(name.trim()))

        /**
         * Removes a credential. Removing the last one is allowed on purpose: the
         * password is the recovery path, so this can never lock the account out.
         */
        suspend fun remove(id: Long) {
            api.remove(id)
        }
    }
