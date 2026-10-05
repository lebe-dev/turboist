package turboist.shrinker

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

class KeepRulesTest {
    @Test
    fun `a plain keep names the class it protects`() {
        assertEquals(
            listOf("ru.tinyops.turboist.core.network.dto.**"),
            KeepRules.classPatterns("-keep class ru.tinyops.turboist.core.network.dto.** { *; }"),
        )
    }

    @Test
    fun `the modifiers a directive may carry do not hide the class`() {
        val configuration =
            """
            -keep,allowobfuscation interface ru.tinyops.turboist.core.network.api.**
            -keep public final class ru.tinyops.turboist.Kept
            -keepnames class ru.tinyops.turboist.Named
            -keepclasseswithmembers class ru.tinyops.turboist.WithMembers {
                kotlinx.serialization.KSerializer serializer(...);
            }
            """.trimIndent()

        assertEquals(
            listOf(
                "ru.tinyops.turboist.core.network.api.**",
                "ru.tinyops.turboist.Kept",
                "ru.tinyops.turboist.Named",
                "ru.tinyops.turboist.WithMembers",
            ),
            KeepRules.classPatterns(configuration),
        )
    }

    @Test
    fun `a rule that only keeps members is not read as protecting the class`() {
        // Keeping a member says what happens to it if the class survives. It is
        // not a reason for the class to survive, so counting it as coverage
        // would let the audit pass on a class the shrinker is free to delete.
        val configuration =
            """
            -keepclassmembers class ru.tinyops.turboist.core.network.** { *** Companion; }
            -keepclassmembernames class ru.tinyops.turboist.core.sync.** { *; }
            """.trimIndent()

        assertEquals(emptyList(), KeepRules.classPatterns(configuration))
    }

    @Test
    fun `options that are not about classes carry no coverage`() {
        val configuration =
            """
            -keepattributes *Annotation*, InnerClasses
            -keepparameternames
            -dontwarn okhttp3.**
            -verbose
            """.trimIndent()

        assertEquals(emptyList(), KeepRules.classPatterns(configuration))
    }

    @Test
    fun `comments and blank lines are ignored`() {
        val configuration =
            """
            # Room finds the generated implementation by name.
            -keep class ru.tinyops.turboist.core.database.*_Impl { *; }   # and nothing else

            """.trimIndent()

        assertEquals(
            listOf("ru.tinyops.turboist.core.database.*_Impl"),
            KeepRules.classPatterns(configuration),
        )
    }

    @Test
    fun `a conditional keep still names the classes it protects`() {
        assertEquals(
            listOf("ru.tinyops.turboist.**"),
            KeepRules.classPatterns("-keep @kotlinx.serialization.Serializable class ru.tinyops.turboist.**"),
        )
    }

    @Test
    fun `a directive naming no class at all is skipped rather than guessed at`() {
        assertNull(KeepRules.classPatterns("-keep").firstOrNull())
    }

    @Test
    fun `two stars span package separators and one star does not`() {
        assertTrue(KeepRules.covers("ru.tinyops.**", "ru.tinyops.turboist.core.network.dto.TaskDto"))
        assertFalse(KeepRules.covers("ru.tinyops.*", "ru.tinyops.turboist.Task"))
        assertTrue(KeepRules.covers("ru.tinyops.turboist.core.database.*_Impl", "ru.tinyops.turboist.core.database.TurboistDatabase_Impl"))
        assertFalse(
            KeepRules.covers(
                "ru.tinyops.turboist.core.database.*_Impl",
                "ru.tinyops.turboist.core.database.entity.TurboistDatabase_Impl",
            ),
        )
    }

    @Test
    fun `a nested class is named with the separator the shrinker uses`() {
        assertTrue(KeepRules.covers("ru.tinyops.Outer\$Inner", "ru.tinyops.Outer\$Inner"))
        assertTrue(KeepRules.covers("ru.tinyops.**", "ru.tinyops.Outer\$Inner"))
    }

    @Test
    fun `the first pattern that names a class is the one reported`() {
        val patterns = listOf("ru.tinyops.other.**", "ru.tinyops.turboist.**", "ru.tinyops.**")
        assertEquals("ru.tinyops.turboist.**", KeepRules.coveringPattern(patterns, "ru.tinyops.turboist.Task"))
        assertNull(KeepRules.coveringPattern(patterns, "com.example.Task"))
    }
}
