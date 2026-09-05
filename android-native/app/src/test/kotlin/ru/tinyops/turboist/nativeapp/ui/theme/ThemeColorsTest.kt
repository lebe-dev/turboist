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
 * Guards the palette against the two ways a hand-written colour scheme goes
 * wrong: drifting off the brand, and pairing a foreground with a background it
 * cannot be read against.
 */
class ThemeColorsTest {
    @Test
    fun `the brand colour is the one the rest of the product uses`() {
        // Same value as the web client's theme colour and the launcher icon.
        assertEquals(0xFFE2580EuL.toLong(), BrandOrange.toLongArgb())
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
    fun `both schemes take their accent from the brand hue`() {
        listOf(TurboistLightColors.primary, TurboistDarkColors.primary).forEach { accent ->
            assertEquals(
                BrandOrange.hue(),
                accent.hue(),
                absoluteTolerance = HUE_TOLERANCE_DEGREES,
            )
        }
    }

    private companion object {
        /** The contrast the accessibility guidelines ask of body text. */
        const val MIN_BODY_CONTRAST = 4.5

        /**
         * How far a tone may sit from the brand hue. Tones of one hue drift a
         * little as they are lightened or darkened; a different hue drifts far
         * more than this.
         */
        const val HUE_TOLERANCE_DEGREES = 12.0
    }
}

/** The foreground/background pairs a screen actually puts text on. */
private fun ColorScheme.readablePairs(): List<Pair<String, Pair<Color, Color>>> =
    listOf(
        "onBackground/background" to (onBackground to background),
        "onSurface/surface" to (onSurface to surface),
        "onSurfaceVariant/surface" to (onSurfaceVariant to surface),
        "onPrimary/primary" to (onPrimary to primary),
        "onPrimaryContainer/primaryContainer" to (onPrimaryContainer to primaryContainer),
        "onSecondary/secondary" to (onSecondary to secondary),
        "onSecondaryContainer/secondaryContainer" to (onSecondaryContainer to secondaryContainer),
        "onTertiary/tertiary" to (onTertiary to tertiary),
        "onTertiaryContainer/tertiaryContainer" to (onTertiaryContainer to tertiaryContainer),
        "onError/error" to (onError to error),
        "onErrorContainer/errorContainer" to (onErrorContainer to errorContainer),
        "inverseOnSurface/inverseSurface" to (inverseOnSurface to inverseSurface),
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
