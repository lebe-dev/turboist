package ru.tinyops.turboist.nativeapp.tasks.ui

import ru.tinyops.turboist.core.model.Label
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * When the label picker offers to make a new label out of what was typed.
 *
 * It must offer exactly when the name is new: offering an existing name would
 * make a twin the server refuses, and not offering a new one leaves the user
 * with no way to make it from the task.
 */
class CreatableLabelNameTest {
    private val known = listOf(Label(localId = 1, name = "Home", createdAt = 0, updatedAt = 0))

    @Test
    fun `nothing typed offers nothing`() {
        assertNull(creatableLabelName("   ", known))
    }

    @Test
    fun `a name no label has is offered, trimmed`() {
        assertEquals("errands", creatableLabelName("  errands ", known))
    }

    @Test
    fun `a name a label already has is not offered, whatever its case`() {
        assertNull(creatableLabelName("home", known))
    }

    @Test
    fun `a part of an existing name is still a new name`() {
        assertEquals("Hom", creatableLabelName("Hom", known))
    }
}
