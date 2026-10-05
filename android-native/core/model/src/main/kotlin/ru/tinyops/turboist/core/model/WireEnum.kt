package ru.tinyops.turboist.core.model

/**
 * A domain enum whose constants have a fixed textual form on the wire and in the
 * replica's columns. The Kotlin constant name is free to read well; [wire] is the
 * contract with the server and must never be renamed.
 */
interface WireEnum {
    val wire: String
}

/**
 * The decoding half of a [WireEnum], implemented by each enum's companion.
 *
 * Decoding is deliberately tolerant: a value this client has never heard of
 * becomes the unknown sentinel instead of throwing. The server can grow an enum
 * while an older build is still installed, and a task carrying a new priority
 * must still be readable, completable and syncable — losing one attribute is a
 * far smaller failure than refusing the whole replica.
 *
 * The sentinel is the single constant with an empty [WireEnum.wire]. It is a
 * read-only value: it is never offered in a picker ([known] excludes it) and must
 * never be written back to the server, which would reject it.
 */
abstract class WireEnumLookup<T>(
    private val all: List<T>,
    private val unknown: T,
) where T : Enum<T>, T : WireEnum {
    /** Every constant a user may choose, in declaration order, sentinel excluded. */
    val known: List<T> = all.filter { it !== unknown }

    /** Tolerant decode: an absent or unrecognised value becomes the sentinel. */
    fun fromWire(raw: String?): T = fromWireOrNull(raw) ?: unknown

    /** Strict decode: an absent or unrecognised value is `null`. */
    fun fromWireOrNull(raw: String?): T? = all.firstOrNull { it !== unknown && it.wire == raw }
}
