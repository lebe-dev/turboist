package ru.tinyops.turboist.nativeapp.quickadd.ui

import android.app.Application
import androidx.compose.foundation.layout.width
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.test.getUnclippedBoundsInRoot
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import org.robolectric.annotation.GraphicsMode
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The date row's layout: choices take the width their wording needs, and fold
 * into two rows instead of being cut short when one row cannot hold them.
 */
@RunWith(RobolectricTestRunner::class)
// Text has no width without the native canvas, and widths are what is under test.
@GraphicsMode(GraphicsMode.Mode.NATIVE)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class SegmentTest {
    @get:Rule
    val compose = createComposeRule()

    private val labels = listOf("No date at all", "Today", "Tomorrow", "Date")

    private fun show(width: Dp) {
        compose.setContent {
            TurboistTheme {
                FittedSegments(
                    segments =
                        labels.map {
                            Segment(label = it, selected = false, container = Color.Gray, content = Color.Black) {}
                        },
                    modifier = Modifier.width(width),
                )
            }
        }
        compose.waitForIdle()
    }

    private fun bounds(label: String) = compose.onNodeWithText(label).getUnclippedBoundsInRoot()

    @Test
    fun `choices that fit share one row, the long one wider than the short one`() {
        show(400.dp)

        val tops = labels.map { bounds(it).top }.toSet()
        assertEquals(1, tops.size)
        val long = bounds(labels[0])
        val short = bounds(labels[3])
        assertTrue(long.right - long.left > short.right - short.left)
    }

    @Test
    fun `choices that do not fit fold into two rows rather than being cut`() {
        show(220.dp)

        assertEquals(bounds(labels[0]).top, bounds(labels[1]).top)
        assertEquals(bounds(labels[2]).top, bounds(labels[3]).top)
        assertTrue(bounds(labels[2]).top > bounds(labels[0]).top)
    }
}
