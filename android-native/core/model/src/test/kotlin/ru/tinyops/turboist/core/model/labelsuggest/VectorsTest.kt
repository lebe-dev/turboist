package ru.tinyops.turboist.core.model.labelsuggest

import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals

class VectorsTest {
    @Test
    fun `a vector is cut to length and scaled to unit length`() {
        val prepared = Vectors.prepare(floatArrayOf(3f, 4f, 100f), dimensions = 2)

        assertContentEquals(floatArrayOf(0.6f, 0.8f), prepared)
        assertEquals(1f, Vectors.dot(prepared, prepared), 1e-6f)
    }

    @Test
    fun `a zero vector stays zero instead of dividing by nothing`() {
        assertContentEquals(floatArrayOf(0f, 0f), Vectors.prepare(floatArrayOf(0f, 0f)))
    }

    @Test
    fun `a stored vector reads back exactly`() {
        val vector = floatArrayOf(0.25f, -1.5f, 3.125f, Float.MIN_VALUE)

        assertContentEquals(vector, Vectors.fromBytes(Vectors.toBytes(vector)))
        assertEquals(16, Vectors.toBytes(vector).size)
    }
}
