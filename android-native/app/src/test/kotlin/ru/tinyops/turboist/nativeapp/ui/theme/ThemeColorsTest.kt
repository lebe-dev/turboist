package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.ui.graphics.Color
import kotlin.math.max
import kotlin.math.min
import kotlin.math.pow
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

/**
 * Guards the palette against the three ways a hand-written colour scheme goes
 * wrong: drifting away from the web client it is a transcription of, losing the
 * layering its surfaces are supposed to express, and pairing a foreground with a
 * background it cannot be read against.
 */
class ThemeColorsTest {
    @Test
    fun `the brand colour is the one the rest of the product uses`() {
        // Same value as the web client's theme colour and the launcher icon.
        assertEquals(0xFFE2580EuL.toLong(), BrandOrange.toLongArgb())
    }

    @Test
    fun `both schemes are the web client's tokens`() {
        // Each expectation is one custom property from the web client's
        // stylesheet, converted to sRGB. A value that changes over there is
        // meant to fail here: two front ends drifting apart is the thing this
        // file exists to prevent.
        // Role to token: background/onSurface are --background/--foreground,
        // surfaceContainerLowest is --card in daylight and --sidebar at night,
        // surfaceVariant is --muted, secondaryContainer is --accent, error is
        // --destructive, and outlineVariant is --border.
        TurboistLightColors.assertRoles(
            "light",
            "background" to 0xFFFBFAF7,
            "onSurface" to 0xFF16100C,
            "surfaceContainerLowest" to 0xFFFFFFFF,
            "primary" to 0xFFEE343B,
            "onPrimary" to 0xFFFAFAFA,
            "surfaceVariant" to 0xFFF3EFEC,
            "onSurfaceVariant" to 0xFF69625D,
            "secondaryContainer" to 0xFFF2EEE9,
            "error" to 0xFFEA1F2F,
            "outlineVariant" to 0xFFE4E1DD,
        )
        TurboistDarkColors.assertRoles(
            "dark",
            "background" to 0xFF100E0C,
            "onSurface" to 0xFFF3F1EE,
            "surfaceContainerLowest" to 0xFF0A0806,
            "primary" to 0xFFFC4447,
            "onPrimary" to 0xFFFCFCFC,
            "surfaceVariant" to 0xFF23201E,
            "onSurfaceVariant" to 0xFF9C9793,
            "secondaryContainer" to 0xFF292624,
            "error" to 0xFFFF5957,
        )
    }

    @Test
    fun `the light scheme is light and the dark scheme is dark`() {
        assertTrue(
            TurboistLightColors.surface.luminance() > TurboistDarkColors.surface.luminance(),
            "the light scheme's surface must be brighter than the dark scheme's",
        )
        assertTrue(TurboistLightColors.surface.luminance() > 0.5)
        assertTrue(TurboistDarkColors.surface.luminance() < 0.1)
    }

    @Test
    fun `every raised surface is a step further from the page`() {
        // Material stacks these five in one order, and a screen relies on that
        // order to look layered: a card must not come out darker than the fill
        // it sits on. Light schemes step down in brightness, dark ones step up.
        TurboistLightColors.surfaceRamp().assertStrictlyOrdered(ascending = false, scheme = "light")
        TurboistDarkColors.surfaceRamp().assertStrictlyOrdered(ascending = true, scheme = "dark")
    }

    @Test
    fun `text is readable on every surface in both schemes`() {
        listOf("light" to TurboistLightColors, "dark" to TurboistDarkColors).forEach { (name, scheme) ->
            scheme.readablePairs().forEach { (label, pair) ->
                val (foreground, background) = pair
                val ratio = contrastRatio(foreground, background)
                assertTrue(
                    ratio >= MIN_BODY_CONTRAST,
                    "$name scheme: $label has contrast $ratio, below $MIN_BODY_CONTRAST",
                )
            }
        }
    }

    @Test
    fun `a label on a filled accent clears the bar for controls`() {
        // The accents are the web client's exact reds, and white on that red is
        // 3.9:1 rather than 4.5:1. Held to the lower bar deliberately: these
        // carry a button's short label, which is what WCAG's 3:1 bar for user
        // interface components is for, and the alternative is a second, darker
        // red that no other part of the product uses.
        listOf("light" to TurboistLightColors, "dark" to TurboistDarkColors).forEach { (name, scheme) ->
            scheme.filledAccentPairs().forEach { (label, pair) ->
                val ratio = contrastRatio(pair.first, pair.second)
                assertTrue(
                    ratio >= MIN_CONTROL_CONTRAST,
                    "$name scheme: $label has contrast $ratio, below $MIN_CONTROL_CONTRAST",
                )
            }
        }
    }

    @Test
    fun `the brand hue survives as the third accent`() {
        // The accent the screens are drawn in is the web client's red, so the
        // product's own orange lives in the one role Material keeps for a third
        // colour. Losing it here would leave the app with no tie to its icon.
        listOf(TurboistLightColors.tertiary, TurboistDarkColors.tertiary).forEach { accent ->
            assertEquals(
                BrandOrange.hue(),
                accent.hue(),
                absoluteTolerance = HUE_TOLERANCE_DEGREES,
            )
        }
    }

    @Test
    fun `the signals are the web client's own`() {
        // The Tailwind steps the web client marks these with, by value. The
        // three priorities are the same at night; the two that are read as words
        // move a step lighter, as the web's `dark:` variants do.
        assertEquals(0xFFFB2C36uL.toLong(), TurboistLightAccents.priorityHigh.toLongArgb())
        assertEquals(0xFFFE9A00uL.toLong(), TurboistLightAccents.priorityMedium.toLongArgb())
        assertEquals(0xFF2B7FFFuL.toLong(), TurboistLightAccents.priorityLow.toLongArgb())
        assertEquals(0xFF009966uL.toLong(), TurboistLightAccents.recurring.toLongArgb())
        assertEquals(0xFFBB4D00uL.toLong(), TurboistLightAccents.pending.toLongArgb())
        assertEquals(TurboistLightAccents.priorityHigh, TurboistDarkAccents.priorityHigh)
        assertEquals(0xFF00D492uL.toLong(), TurboistDarkAccents.recurring.toLongArgb())
        assertEquals(0xFFFFD230uL.toLong(), TurboistDarkAccents.pending.toLongArgb())
    }

    @Test
    fun `the queued-write badge is a tint of the page, not a second surface`() {
        listOf(TurboistLightAccents, TurboistDarkAccents).forEach { accents ->
            assertTrue(
                accents.pendingContainer.alpha < 1f,
                "the badge's backdrop must be translucent so it reads as a tint",
            )
            assertEquals(
                TurboistLightAccents.priorityMedium.toLongArgb(),
                accents.pendingContainer.copy(alpha = 1f).toLongArgb(),
                "the backdrop is the same amber the badge's text is a deeper step of",
            )
        }
    }

    @Test
    fun `the signals that are read as words are readable`() {
        // A mark drawn as a ring or a glyph carries at 3:1; these two are set in
        // text, so they are held to the body-text bar against the page they are
        // written on.
        listOf(
            "light" to (TurboistLightAccents to TurboistLightColors),
            "dark" to (TurboistDarkAccents to TurboistDarkColors),
        ).forEach { (name, pair) ->
            val (accents, scheme) = pair
            val ratio = contrastRatio(accents.pending, scheme.surface)
            assertTrue(ratio >= MIN_BODY_CONTRAST, "$name scheme: queued-write text is $ratio on the page")
        }
    }

    private companion object {
        /** The contrast the accessibility guidelines ask of body text. */
        const val MIN_BODY_CONTRAST = 4.5

        /** What the same guidelines ask of a control's own colours. */
        const val MIN_CONTROL_CONTRAST = 3.0

        /**
         * How far a tone may sit from the brand hue. Tones of one hue drift a
         * little as they are lightened or darkened; a different hue drifts far
         * more than this.
         */
        const val HUE_TOLERANCE_DEGREES = 12.0
    }
}

/** Checks a scheme's roles against the values they were transcribed from. */
private fun ColorScheme.assertRoles(
    scheme: String,
    vararg expected: Pair<String, Long>,
) {
    val roles: Map<String, Color> =
        mapOf(
            "background" to background,
            "onSurface" to onSurface,
            "surfaceContainerLowest" to surfaceContainerLowest,
            "primary" to primary,
            "onPrimary" to onPrimary,
            "surfaceVariant" to surfaceVariant,
            "onSurfaceVariant" to onSurfaceVariant,
            "secondaryContainer" to secondaryContainer,
            "error" to error,
            "outlineVariant" to outlineVariant,
        )
    expected.forEach { (role, value) ->
        assertEquals(
            value,
            roles.getValue(role).toLongArgb(),
            "$scheme scheme: $role",
        )
    }
}

/** The five surfaces Material stacks, from the page outwards. */
private fun ColorScheme.surfaceRamp(): List<Pair<String, Color>> =
    listOf(
        "surfaceContainerLowest" to surfaceContainerLowest,
        "surfaceContainerLow" to surfaceContainerLow,
        "surfaceContainer" to surfaceContainer,
        "surfaceContainerHigh" to surfaceContainerHigh,
        "surfaceContainerHighest" to surfaceContainerHighest,
    )

private fun List<Pair<String, Color>>.assertStrictlyOrdered(
    ascending: Boolean,
    scheme: String,
) {
    zipWithNext().forEach { (lower, higher) ->
        val moved = higher.second.luminance() - lower.second.luminance()
        assertTrue(
            if (ascending) moved > 0 else moved < 0,
            "$scheme scheme: ${higher.first} does not step past ${lower.first}",
        )
    }
}

/** The foreground/background pairs a screen actually puts text on. */
private fun ColorScheme.readablePairs(): List<Pair<String, Pair<Color, Color>>> =
    listOf(
        "onBackground/background" to (onBackground to background),
        "onSurface/surface" to (onSurface to surface),
        "onSurfaceVariant/surface" to (onSurfaceVariant to surface),
        "onPrimaryContainer/primaryContainer" to (onPrimaryContainer to primaryContainer),
        "onSecondary/secondary" to (onSecondary to secondary),
        "onSecondaryContainer/secondaryContainer" to (onSecondaryContainer to secondaryContainer),
        "onTertiaryContainer/tertiaryContainer" to (onTertiaryContainer to tertiaryContainer),
        "onErrorContainer/errorContainer" to (onErrorContainer to errorContainer),
        "inverseOnSurface/inverseSurface" to (inverseOnSurface to inverseSurface),
    )

/** The pairs that are a filled control rather than text on a page. */
private fun ColorScheme.filledAccentPairs(): List<Pair<String, Pair<Color, Color>>> =
    listOf(
        "onPrimary/primary" to (onPrimary to primary),
        "onError/error" to (onError to error),
        "onTertiary/tertiary" to (onTertiary to tertiary),
    )

private fun Color.toLongArgb(): Long {
    fun channel(value: Float): Long = (value * 255f + 0.5f).toLong()
    return (channel(alpha) shl 24) or (channel(red) shl 16) or (channel(green) shl 8) or channel(blue)
}

/** Relative luminance as defined by the web contrast formula. */
private fun Color.luminance(): Double {
    fun linear(channel: Float): Double {
        val c = channel.toDouble()
        return if (c <= 0.03928) c / 12.92 else ((c + 0.055) / 1.055).pow(2.4)
    }
    return 0.2126 * linear(red) + 0.7152 * linear(green) + 0.0722 * linear(blue)
}

private fun contrastRatio(
    a: Color,
    b: Color,
): Double {
    val lighter = max(a.luminance(), b.luminance())
    val darker = min(a.luminance(), b.luminance())
    return (lighter + 0.05) / (darker + 0.05)
}

/** Hue in degrees, 0 for a colour with no saturation left to speak of. */
private fun Color.hue(): Double {
    val r = red.toDouble()
    val g = green.toDouble()
    val b = blue.toDouble()
    val maxComponent = maxOf(r, g, b)
    val minComponent = minOf(r, g, b)
    val delta = maxComponent - minComponent
    if (delta == 0.0) return 0.0
    val raw =
        when (maxComponent) {
            r -> 60.0 * (((g - b) / delta) % 6.0)
            g -> 60.0 * (((b - r) / delta) + 2.0)
            else -> 60.0 * (((r - g) / delta) + 4.0)
        }
    return (raw + 360.0) % 360.0
}
