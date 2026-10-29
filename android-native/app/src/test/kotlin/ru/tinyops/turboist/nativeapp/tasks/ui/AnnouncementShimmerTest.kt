package ru.tinyops.turboist.nativeapp.tasks.ui

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/** Where the light runs along the announcement while it shimmers, and how it fades. */
class AnnouncementShimmerTest {
    private val width = 100f

    @Test
    fun `the light starts off the left edge`() {
        val center = shimmerCenter(progress = 0f, width = width)
        assertTrue(center + width / 2 <= 0f, "the band must be wholly off screen, its centre is at $center")
    }

    @Test
    fun `the light ends off the right edge`() {
        val center = shimmerCenter(progress = 1f, width = width)
        assertTrue(center - width / 2 >= width, "the band must be wholly off screen, its centre is at $center")
    }

    @Test
    fun `the light crosses the middle while the sweep is under way`() {
        assertEquals(width / 2, shimmerCenter(progress = 0.4f, width = width), absoluteTolerance = 0.001f)
    }

    @Test
    fun `the outline holds its strength for most of the sweep`() {
        assertEquals(SHIMMER_PEAK_ALPHA, shimmerAlpha(0f))
        assertEquals(SHIMMER_PEAK_ALPHA, shimmerAlpha(0.88f))
    }

    @Test
    fun `the outline fades out at the end`() {
        assertEquals(0f, shimmerAlpha(1f))
        val midFade = shimmerAlpha(0.94f)
        assertTrue(midFade > 0f && midFade < SHIMMER_PEAK_ALPHA, "half way through the fade it is $midFade")
    }
}
