package ru.tinyops.turboist.nativeapp.harpoon

import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import kotlin.test.Test
import kotlin.test.assertEquals

/**
 * The rule of the jump pair, which this device applies to a tap and the server
 * applies to the request that follows it.
 *
 * Both sides have to reach the same two entries from the same taps, or the pair
 * would change under the user the next time the device caught up. The rule is
 * small and exact: two slots, the older one out first, hooking on something
 * already in the pair moves it rather than duplicating it, and unhooking
 * something that is not in it changes nothing.
 */
class HarpoonPairTest {
    private fun task(localId: Long) = HarpoonEntry(HarpoonTarget.TASK, localId)

    private fun project(localId: Long) = HarpoonEntry(HarpoonTarget.PROJECT, localId)

    @Test
    fun `the pair holds one of each kind, in the order they were hooked on`() {
        val pair = withHarpooned(withHarpooned(emptyList(), task(1)), project(2))

        assertEquals(listOf(task(1), project(2)), pair)
    }

    @Test
    fun `hooking the same thing on twice leaves one entry, not two`() {
        val pair = withHarpooned(withHarpooned(emptyList(), task(1)), task(1))

        assertEquals(listOf(task(1)), pair)
    }

    @Test
    fun `a third thing evicts the older of the two`() {
        val filled = withHarpooned(withHarpooned(emptyList(), task(1)), task(2))

        val pair = withHarpooned(filled, task(3))

        assertEquals(listOf(task(2), task(3)), pair)
    }

    @Test
    fun `hooking on the older of the two makes it the newer one`() {
        val filled = withHarpooned(withHarpooned(emptyList(), task(1)), task(2))

        val pair = withHarpooned(filled, task(1))

        assertEquals(listOf(task(2), task(1)), pair)
    }

    @Test
    fun `a task and a project with the same id are different things`() {
        val pair = withHarpooned(withHarpooned(emptyList(), task(1)), project(1))

        assertEquals(listOf(task(1), project(1)), pair)
    }

    @Test
    fun `unhooking removes exactly one end and leaves the other`() {
        val filled = withHarpooned(withHarpooned(emptyList(), task(1)), project(2))

        assertEquals(listOf(project(2)), withoutHarpooned(filled, task(1)))
    }

    @Test
    fun `unhooking something that is not in the pair changes nothing`() {
        val filled = withHarpooned(emptyList(), task(1))

        assertEquals(listOf(task(1)), withoutHarpooned(filled, task(9)))
    }
}
