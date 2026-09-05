package ru.tinyops.turboist.nativeapp.account

import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.sync.write.OpNames
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * The account's administrative surfaces never reach storage, and stay that way.
 *
 * This is the property of the feature that cannot be seen by looking at a
 * screen, so it is asserted rather than assumed. Three kinds of payload cross
 * these screens and none of them may be written down:
 *
 * - a session list, because a stale one answers "who can reach my account" with
 *   a device the user already revoked;
 * - a token's plaintext, which the server itself does not keep — a copy on the
 *   device would be the only one, revocable by nobody who noticed it leaked;
 * - an enrolment secret and the recovery codes, which are the second factor. A
 *   copy of them on the phone that holds the first one is not a second factor.
 *
 * The behaviour is already covered by the view models' own checks, which run
 * against nothing but a scripted server. What those cannot show is that a later
 * change will not quietly wire this layer to a database, a queue or a
 * preference file — which is what is checked below.
 */
class AccountSecretsStayOffDiskTest {
    /** Anything that would put a value somewhere it survives the screen. */
    private val storagePackages =
        listOf(
            "ru.tinyops.turboist.core.database",
            "ru.tinyops.turboist.core.sync",
            "androidx.datastore",
            "android.content.SharedPreferences",
            "java.io.File",
        )

    /**
     * Everything that handles one of the three payloads.
     *
     * The session control is deliberately absent: it is the one part of the
     * layer allowed to touch the replica, because signing this device out counts
     * and then empties it — and it is the counting and the emptying it reaches
     * for, never a session list. Its own check is the second one below.
     */
    private val payloadHandlers =
        listOf(
            SessionsViewModel::class.java,
            ApiTokensViewModel::class.java,
            TwoFactorViewModel::class.java,
            HttpApiTokenControl::class.java,
            HttpTwoFactorControl::class.java,
        )

    @Test
    fun `nothing that handles an account payload holds a place to write one`() {
        for (type in payloadHandlers) {
            val held = type.declaredFields.map { it.type.name }
            for (name in held) {
                assertTrue(
                    storagePackages.none { name.startsWith(it) },
                    "${type.simpleName} holds $name, which is somewhere a value could be written down",
                )
            }
        }
    }

    @Test
    fun `the state each screen renders carries only what the server just said`() {
        // The payloads live in the UI state and nowhere else, so the state
        // classes are checked too: a field pointing at a store here would be a
        // copy of a secret with a lifetime longer than the screen.
        val states =
            listOf(
                SessionsUiState::class.java,
                ApiTokensUiState::class.java,
                TwoFactorUiState::class.java,
            )
        for (type in states) {
            val held = type.declaredFields.map { it.type.name }
            for (name in held) {
                assertTrue(
                    storagePackages.none { name.startsWith(it) },
                    "${type.simpleName} holds $name, which outlives the screen it is on",
                )
            }
        }
    }

    @Test
    fun `none of these is one of the records a catch-up applies`() {
        val administrative = listOf("session", "token", "totp", "passkey", "recovery")
        assertTrue(
            ReplicaEntityKind.entries.none { kind ->
                administrative.any { kind.stored.contains(it, ignoreCase = true) }
            },
            "an administrative record has no place among the entities the change history reports",
        )
    }

    @Test
    fun `there is no queued write that would send one of these anywhere`() {
        val ops =
            OpNames::class.java.declaredFields
                .filter { it.type == String::class.java }
                .map { field ->
                    field.isAccessible = true
                    field.get(OpNames) as String
                }
        assertTrue(ops.isNotEmpty(), "the catalogue of writes should not be empty; the check would prove nothing")
        // Every action on these screens needs the server to agree at the moment
        // it is taken — revoking a session offline and sending it later would
        // reassure the user about a device that stayed signed in for a week.
        val administrative = listOf("session", "token", "totp", "twofactor", "logout")
        assertTrue(
            ops.none { op -> administrative.any { op.contains(it, ignoreCase = true) } },
            "these surfaces are online-only, so no queued write of one can exist",
        )
    }
}
