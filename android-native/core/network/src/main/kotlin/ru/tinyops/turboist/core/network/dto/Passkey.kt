package ru.tinyops.turboist.core.network.dto

import kotlinx.serialization.EncodeDefault
import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject
import ru.tinyops.turboist.core.model.ClientKind
import ru.tinyops.turboist.core.network.TurboistJson

/**
 * One credential the account can sign in with, as the management screen shows it.
 *
 * The credential itself never leaves the server — a public key is of no use to
 * the client — so this is a label and two timestamps and nothing more.
 */
@Serializable
data class PasskeyDto(
    val id: Long = 0,
    val name: String = "",
    val createdAt: String = "",
    /** Absent until the credential has actually signed someone in. */
    val lastUsedAt: String? = null,
)

/**
 * A ceremony the server has started: the challenge it is holding, and the
 * options the authenticator has to answer it with.
 *
 * The challenge lives on the server for a few minutes and is single-use, so
 * [ceremonyId] is the only thing that ties the answer back to it. The options
 * are carried as raw JSON rather than as typed fields on purpose: they are
 * defined by the platform, not by this app, and re-shaping them through a data
 * class is how a member the server added would get silently dropped on the way
 * to the authenticator.
 */
@Serializable
data class PasskeyCeremonyDto(
    val ceremonyId: String = "",
    val options: JsonObject = JsonObject(emptyMap()),
)

/**
 * The options as the platform credential API takes them.
 *
 * The server sends the shape a browser wants — the real options wrapped in a
 * `publicKey` member, which is the argument `navigator.credentials` is called
 * with — while the platform API on this device takes the inner object on its
 * own. Unwrapping happens here so exactly one place knows about the difference;
 * options that arrive unwrapped are passed through, so a server that ever stops
 * wrapping them still works.
 *
 * Nothing is decoded on the way: every binary member (the challenge, the user
 * handle, credential ids) stays the base64url string the server wrote. Decoding
 * and re-encoding is what loses the URL alphabet or adds padding, and the
 * failure that follows is a signature that verifies nowhere.
 *
 * @throws IllegalArgumentException when there are no options to hand over, which
 *   is a server this client cannot run a ceremony against.
 */
fun PasskeyCeremonyDto.optionsJson(): String {
    val wrapped = options["publicKey"]
    val ceremony = if (wrapped is JsonObject) wrapped else options
    require(ceremony.isNotEmpty()) { "the server started a passkey ceremony without any options" }
    return TurboistJson.encodeToString(JsonObject.serializer(), ceremony)
}

/**
 * Reads what the authenticator answered, ready to be posted back.
 *
 * The platform produces the credential already in the JSON form the server
 * verifies, so it is parsed and forwarded rather than mapped: the signature is
 * over bytes the server reconstructs from these exact members, and anything this
 * client rewrote on the way would break the verification it is asking for.
 *
 * @throws IllegalArgumentException when the answer is not a credential object.
 */
fun passkeyCredentialOf(responseJson: String): JsonObject {
    val answer = TurboistJson.parseToJsonElement(responseJson)
    require(answer is JsonObject) { "the authenticator answered with something other than a credential" }
    return answer
}

/**
 * Starts a discoverable login.
 *
 * There is no username: the credential is resident on the device, so the
 * authenticator reports which account it holds and the server resolves the rest.
 */
@Serializable
data class PasskeyLoginBeginRequest(
    /** Written to the wire always, for the reason given on [SetupRequest.clientKind]. */
    @EncodeDefault(EncodeDefault.Mode.ALWAYS)
    @OptIn(ExperimentalSerializationApi::class)
    val clientKind: String = ClientKind.ANDROID.wire,
)

/**
 * Finishes a discoverable login, and answers with a finished session.
 *
 * Never with a second-factor challenge: the authenticator holds the key *and*
 * verified the user, so the assertion is already two factors and a code step
 * after it would add friction without adding one.
 */
@Serializable
data class PasskeyLoginFinishRequest(
    val ceremonyId: String,
    val credential: JsonObject,
    /** Written to the wire always, for the reason given on [SetupRequest.clientKind]. */
    @EncodeDefault(EncodeDefault.Mode.ALWAYS)
    @OptIn(ExperimentalSerializationApi::class)
    val clientKind: String = ClientKind.ANDROID.wire,
)

/**
 * Stores a newly created credential.
 *
 * [name] is only a label, and an absent one leaves the naming to the server
 * rather than sending a placeholder this client made up.
 */
@Serializable
data class PasskeyRegisterFinishRequest(
    val ceremonyId: String,
    val credential: JsonObject,
    val name: String? = null,
)

/** Relabels a stored credential. */
@Serializable
data class PasskeyRenameRequest(
    val name: String,
)
