package ru.tinyops.turboist.core.model.labelsuggest

import java.nio.ByteBuffer
import java.nio.ByteOrder
import kotlin.math.sqrt

/** The little arithmetic suggestions need on embeddings. */
object Vectors {
    /**
     * Cuts a raw embedding down to [dimensions] and scales it to unit length,
     * after which the dot product of two vectors is their cosine similarity.
     */
    fun prepare(
        raw: FloatArray,
        dimensions: Int = LabelSuggestionRecipe.DIMENSIONS,
    ): FloatArray {
        val kept = if (raw.size > dimensions) raw.copyOf(dimensions) else raw.copyOf()
        var sum = 0.0
        for (value in kept) sum += value.toDouble() * value
        val norm = sqrt(sum).toFloat()
        if (norm == 0f) return kept
        for (i in kept.indices) kept[i] /= norm
        return kept
    }

    fun dot(
        a: FloatArray,
        b: FloatArray,
    ): Float {
        var sum = 0f
        for (i in 0 until minOf(a.size, b.size)) sum += a[i] * b[i]
        return sum
    }

    /** A vector as stored: little-endian 32-bit floats. */
    fun toBytes(vector: FloatArray): ByteArray {
        val buffer = ByteBuffer.allocate(vector.size * Float.SIZE_BYTES).order(ByteOrder.LITTLE_ENDIAN)
        buffer.asFloatBuffer().put(vector)
        return buffer.array()
    }

    fun fromBytes(bytes: ByteArray): FloatArray {
        val floats = ByteBuffer.wrap(bytes).order(ByteOrder.LITTLE_ENDIAN).asFloatBuffer()
        return FloatArray(floats.remaining()).also { floats.get(it) }
    }
}
