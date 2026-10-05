package ru.tinyops.turboist.core.network

import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.descriptors.SerialDescriptor
import kotlinx.serialization.encoding.Decoder
import kotlinx.serialization.encoding.Encoder
import kotlinx.serialization.json.JsonDecoder
import kotlinx.serialization.json.JsonEncoder
import kotlinx.serialization.json.JsonNull

/**
 * A PATCH field that can be set to a value or emptied, told apart from a field
 * that is not being touched at all.
 *
 * The API reads three states out of a PATCH body: a key that is absent leaves
 * the field unchanged, an explicit `null` clears it, and a value sets it. A plain
 * nullable Kotlin property can only express two of those, which is how a request
 * meant to clear a due date ends up leaving it in place.
 *
 * So a clearable field is declared as `Clearable<T>? = null`: the outer `null`
 * is "not touching it" and is dropped from the body, [Clear] is written as JSON
 * null, and [Set] carries the new value.
 */
@Serializable(with = ClearableSerializer::class)
sealed interface Clearable<out T : Any> {
    /** Write this value. */
    data class Set<T : Any>(val value: T) : Clearable<T>

    /** Empty the field. Serialized as an explicit JSON null. */
    data object Clear : Clearable<Nothing>

    companion object {
        /** [Set] for a value, [Clear] for `null` — the shape most call sites already have. */
        fun <T : Any> of(value: T?): Clearable<T> = if (value == null) Clear else Set(value)
    }
}

/**
 * Encodes [Clearable] as "the value, or JSON null".
 *
 * The null has to be written as a JSON element rather than through
 * `encodeNull()`: the property being serialized is not nullable at the Kotlin
 * level — its nullability is the *outer* "absent" state — so the generic encoder
 * would refuse. This layer only ever speaks JSON, so reaching for the JSON
 * encoder is honest rather than a shortcut.
 */
class ClearableSerializer<T : Any>(
    private val valueSerializer: KSerializer<T>,
) : KSerializer<Clearable<T>> {
    override val descriptor: SerialDescriptor = valueSerializer.descriptor

    override fun serialize(
        encoder: Encoder,
        value: Clearable<T>,
    ) {
        when (value) {
            is Clearable.Set -> valueSerializer.serialize(encoder, value.value)
            Clearable.Clear -> {
                val json =
                    encoder as? JsonEncoder
                        ?: throw IllegalStateException("a clearable field can only be written to JSON")
                json.encodeJsonElement(JsonNull)
            }
        }
    }

    override fun deserialize(decoder: Decoder): Clearable<T> {
        val json =
            decoder as? JsonDecoder
                ?: throw IllegalStateException("a clearable field can only be read from JSON")
        val element = json.decodeJsonElement()
        if (element is JsonNull) return Clearable.Clear
        return Clearable.Set(json.json.decodeFromJsonElement(valueSerializer, element))
    }
}
