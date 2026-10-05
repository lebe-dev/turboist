package ru.tinyops.turboist.nativeapp.projects

import androidx.compose.ui.graphics.Color
import ru.tinyops.turboist.nativeapp.projects.ui.projectTint
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotNull
import kotlin.test.assertNull

/**
 * The colour a project is drawn in.
 *
 * The server accepts one of ten names or a `#rrggbb` value and nothing else, so
 * those are the two shapes worth reading. Anything else is treated as no colour
 * rather than guessed at: a newer server may accept a spelling this build does
 * not know, and painting a project a colour the user never chose is worse than
 * leaving it in the theme's own.
 */
class ProjectTintTest {
    @Test
    fun `a named colour is one of the ten the server accepts`() {
        assertNotNull(projectTint("blue"))
        assertEquals(projectTint("blue"), projectTint("BLUE"), "the name is not case sensitive")
    }

    @Test
    fun `a hex value is read as fully opaque`() {
        assertEquals(Color(0xFF102030), projectTint("#102030"))
    }

    @Test
    fun `a project with no colour is left in the theme's own`() {
        assertNull(projectTint(""))
        assertNull(projectTint("   "))
    }

    @Test
    fun `a value this build cannot read is treated as no colour at all`() {
        assertNull(projectTint("chartreuse"))
        assertNull(projectTint("#12345"))
        assertNull(projectTint("#gggggg"))
    }
}
