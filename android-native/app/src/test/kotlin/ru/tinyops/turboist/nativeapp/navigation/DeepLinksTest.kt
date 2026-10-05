package ru.tinyops.turboist.nativeapp.navigation

import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class DeepLinksTest {
    @Test
    fun `a task link is the app scheme followed by the server id`() {
        assertEquals("turboist://task/42", DeepLinks.task(42L))
    }

    @Test
    fun `task links sit under the base path`() {
        assertTrue(DeepLinks.task(1L).startsWith("${DeepLinks.TASK_BASE_PATH}/"))
    }

    /**
     * The constant and the manifest filter are two halves of one thing: the
     * system only hands the app a link the filter claims, and the graph only
     * matches a link the constant spells. Split them and an incoming link reaches
     * the activity and lands nowhere, which nothing else would notice. The
     * manifest is therefore read rather than transcribed.
     */
    @Test
    fun `the manifest claims the scheme and host the base path is built from`() {
        val declared = manifestDeepLinkPrefixes()

        assertTrue(
            DeepLinks.TASK_BASE_PATH in declared,
            "the manifest declares $declared, which does not include ${DeepLinks.TASK_BASE_PATH}",
        )
    }

    /** Every `scheme://host` the launcher activity's intent filters claim. */
    private fun manifestDeepLinkPrefixes(): Set<String> {
        val document =
            DocumentBuilderFactory
                .newInstance()
                .apply { isNamespaceAware = true }
                .newDocumentBuilder()
                .parse(manifestFile())
        val data = document.getElementsByTagName("data")
        return (0 until data.length)
            .map { data.item(it) as Element }
            .mapNotNull { element ->
                val scheme = element.getAttributeNS(ANDROID_NAMESPACE, "scheme").orEmpty()
                val host = element.getAttributeNS(ANDROID_NAMESPACE, "host").orEmpty()
                if (scheme.isEmpty() || host.isEmpty()) null else "$scheme://$host"
            }.toSet()
    }

    /**
     * Tests run from the module directory, but that is a convention rather than a
     * guarantee, so the repository copy is looked for as well and a miss fails
     * loudly instead of quietly asserting over nothing.
     */
    private fun manifestFile(): File {
        val candidates =
            listOf(
                File("src/main/AndroidManifest.xml"),
                File("app/src/main/AndroidManifest.xml"),
                File("android-native/app/src/main/AndroidManifest.xml"),
            )
        return candidates.firstOrNull { it.isFile }
            ?: error("the application manifest was not found next to ${File(".").absolutePath}")
    }

    private companion object {
        const val ANDROID_NAMESPACE = "http://schemas.android.com/apk/res/android"
    }
}
