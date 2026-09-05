package ru.tinyops.turboist.nativeapp.harpoon.ui

import android.app.Application
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.junit4.createComposeRule
import androidx.compose.ui.test.onAllNodesWithContentDescription
import androidx.compose.ui.test.onNodeWithContentDescription
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.sync.write.HarpoonTarget
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonEntry
import ru.tinyops.turboist.nativeapp.harpoon.HarpoonJump
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * The jump control, as it is actually drawn.
 *
 * It is the whole of the pair's surface — where the two things are jumped to,
 * and where the thing on screen is hooked on and taken off — so what is checked
 * here is that each of those is reachable and says the right thing, and that a
 * control with nothing to offer is not drawn at all.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], qualifiers = "w411dp-h891dp", application = Application::class)
class HarpoonActionTest {
    @get:Rule
    val compose = createComposeRule()

    private val website = HarpoonEntry(HarpoonTarget.PROJECT, 7)
    private val shipIt = HarpoonEntry(HarpoonTarget.TASK, 5)

    private fun text(
        resId: Int,
        vararg arguments: Any,
    ): String = RuntimeEnvironment.getApplication().getString(resId, *arguments)

    private fun show(
        current: HarpoonEntry?,
        jumps: List<HarpoonJump>,
        onToggle: (HarpoonEntry) -> Unit = {},
        onOpen: (HarpoonEntry) -> Unit = {},
    ) {
        compose.setContent {
            TurboistTheme {
                HarpoonAction(current = current, jumps = jumps, onToggle = onToggle, onOpen = onOpen)
            }
        }
    }

    private fun openMenu() {
        compose.onNodeWithContentDescription(text(R.string.harpoon_attach)).performClick()
    }

    @Test
    fun `both ends of the pair are offered by name and jumping opens the one picked`() {
        val opened = mutableListOf<HarpoonEntry>()
        show(
            current = null,
            jumps = listOf(HarpoonJump(website, "Website"), HarpoonJump(shipIt, "Ship it")),
            onOpen = { opened += it },
        )

        openMenu()
        compose.onNodeWithText(text(R.string.harpoon_jumpTo, "Website")).assertIsDisplayed()
        compose.onNodeWithText(text(R.string.harpoon_jumpTo, "Ship it")).performClick()

        assertEquals(listOf(shipIt), opened)
    }

    @Test
    fun `what is on screen and off the pair is offered to be hooked on`() {
        val toggled = mutableListOf<HarpoonEntry>()
        show(current = website, jumps = emptyList(), onToggle = { toggled += it })

        openMenu()
        compose.onNodeWithText(text(R.string.harpoon_attach)).performClick()

        assertEquals(listOf(website), toggled)
    }

    @Test
    fun `what is on screen and already on the pair is offered to be taken off`() {
        val toggled = mutableListOf<HarpoonEntry>()
        show(
            current = website,
            jumps = listOf(HarpoonJump(website, "Website")),
            onToggle = { toggled += it },
        )

        openMenu()
        compose.onNodeWithText(text(R.string.harpoon_detach)).performClick()

        assertEquals(listOf(website), toggled)
    }

    @Test
    fun `a control with nothing to jump to and nothing to hook on is not drawn`() {
        show(current = null, jumps = emptyList())

        assertTrue(
            compose
                .onAllNodesWithContentDescription(text(R.string.harpoon_attach))
                .fetchSemanticsNodes()
                .isEmpty(),
            "the control was drawn with nothing to offer",
        )
    }
}
