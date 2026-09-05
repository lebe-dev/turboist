package ru.tinyops.turboist.nativeapp.quickadd

import android.app.Application
import android.content.Intent
import android.net.Uri
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.annotation.Config
import ru.tinyops.turboist.nativeapp.navigation.DeepLinks
import kotlin.test.assertEquals
import kotlin.test.assertNull

/**
 * What arrives from outside the app, and which of it is a capture.
 *
 * Two things are: the launcher's own shortcut, which asks for an empty sheet,
 * and another app sharing text, which asks for one filled in. Everything else
 * has to be left alone — a task link in particular, because reading it as a
 * capture would swallow the link and open a blank task instead of the one it
 * names.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class ShareTargetTest {
    @Test
    fun `the launcher shortcut asks for an empty sheet`() {
        val intent = Intent(ACTION_QUICK_ADD)

        assertEquals(QuickAddRequest(), intent.quickAddRequest())
    }

    @Test
    fun `shared text arrives as a draft`() {
        val intent =
            Intent(Intent.ACTION_SEND).apply {
                type = "text/plain"
                putExtra(Intent.EXTRA_SUBJECT, "Kotlin coroutines guide")
                putExtra(Intent.EXTRA_TEXT, "https://example.test/guide")
            }

        assertEquals(
            QuickAddRequest(title = "Kotlin coroutines guide", description = "https://example.test/guide"),
            intent.quickAddRequest(),
        )
    }

    @Test
    fun `a share with nothing in it still opens the sheet`() {
        val intent = Intent(Intent.ACTION_SEND).apply { type = "text/plain" }

        assertEquals(QuickAddRequest(), intent.quickAddRequest())
    }

    /**
     * The app stores no attachments, so a shared file has nothing it could keep.
     * Turning one into an empty task would silently drop what was shared.
     */
    @Test
    fun `a shared file is not a capture`() {
        val intent =
            Intent(Intent.ACTION_SEND).apply {
                type = "image/png"
                putExtra(Intent.EXTRA_TEXT, "holiday.png")
            }

        assertNull(intent.quickAddRequest())
    }

    @Test
    fun `a task link is not a capture`() {
        val intent = Intent(Intent.ACTION_VIEW, Uri.parse(DeepLinks.task(4242L)))

        assertNull(intent.quickAddRequest())
    }

    @Test
    fun `plainly opening the app is not a capture`() {
        val intent = Intent(Intent.ACTION_MAIN).apply { addCategory(Intent.CATEGORY_LAUNCHER) }

        assertNull(intent.quickAddRequest())
    }
}
