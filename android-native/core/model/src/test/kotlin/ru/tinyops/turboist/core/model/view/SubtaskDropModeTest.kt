package ru.tinyops.turboist.core.model.view

import kotlin.test.Test
import kotlin.test.assertEquals

class SubtaskDropModeTest {
    @Test
    fun `the top and bottom quarter of a row read as dependency`() {
        assertEquals(SubtaskDropMode.DEPENDENCY, subtaskDropMode(0f))
        assertEquals(SubtaskDropMode.DEPENDENCY, subtaskDropMode(0.1f))
        assertEquals(SubtaskDropMode.DEPENDENCY, subtaskDropMode(0.9f))
        assertEquals(SubtaskDropMode.DEPENDENCY, subtaskDropMode(1f))
    }

    @Test
    fun `the middle half of a row reads as nest`() {
        assertEquals(SubtaskDropMode.NEST, subtaskDropMode(0.25f))
        assertEquals(SubtaskDropMode.NEST, subtaskDropMode(0.5f))
        assertEquals(SubtaskDropMode.NEST, subtaskDropMode(0.75f))
    }
}
