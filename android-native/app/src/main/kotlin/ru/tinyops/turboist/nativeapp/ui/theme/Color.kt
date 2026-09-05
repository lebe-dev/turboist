package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color

/**
 * The product's brand orange. Every other accent tone in this file is a tone of
 * the same hue, so the app reads as the same product as the web client and the
 * launcher icon no matter which scheme is in play.
 */
val BrandOrange: Color = Color(0xFFE2580E)

// Tonal steps of the brand hue, named after their Material tone (0 = black,
// 100 = white). A light scheme takes its accent from the dark end so text on it
// can be white; a dark scheme takes it from the light end for the same reason.
private val BrandTone10 = Color(0xFF3A0B00)
private val BrandTone20 = Color(0xFF5E1600)
private val BrandTone30 = Color(0xFF832400)
private val BrandTone40 = Color(0xFFA63A00)
private val BrandTone80 = Color(0xFFFFB599)
private val BrandTone90 = Color(0xFFFFDBCE)

// Warm neutrals: the greys are tinted towards the brand hue so surfaces next to
// an orange accent do not look blue by comparison.
private val NeutralTone10 = Color(0xFF201A18)
private val NeutralTone20 = Color(0xFF362F2D)
private val NeutralTone90 = Color(0xFFEDE0DD)
private val NeutralTone95 = Color(0xFFFBEEEB)
private val NeutralTone99 = Color(0xFFFFFBFF)

private val NeutralVariant30 = Color(0xFF53433F)
private val NeutralVariant50 = Color(0xFF85736E)
private val NeutralVariant60 = Color(0xFFA08C87)
private val NeutralVariant80 = Color(0xFFD8C2BC)
private val NeutralVariant90 = Color(0xFFF5DED8)

/** Light scheme. Accent tones sit dark enough to carry white text. */
val TurboistLightColors: ColorScheme =
    lightColorScheme(
        primary = BrandTone40,
        onPrimary = Color.White,
        primaryContainer = BrandTone90,
        onPrimaryContainer = BrandTone10,
        inversePrimary = BrandTone80,
        secondary = Color(0xFF77574B),
        onSecondary = Color.White,
        secondaryContainer = Color(0xFFFFDBCE),
        onSecondaryContainer = Color(0xFF2C160D),
        tertiary = Color(0xFF6C5D2F),
        onTertiary = Color.White,
        tertiaryContainer = Color(0xFFF6E1A6),
        onTertiaryContainer = Color(0xFF241A00),
        error = Color(0xFFBA1A1A),
        onError = Color.White,
        errorContainer = Color(0xFFFFDAD6),
        onErrorContainer = Color(0xFF410002),
        background = NeutralTone99,
        onBackground = NeutralTone10,
        surface = NeutralTone99,
        onSurface = NeutralTone10,
        surfaceVariant = NeutralVariant90,
        onSurfaceVariant = NeutralVariant30,
        outline = NeutralVariant50,
        outlineVariant = NeutralVariant80,
        inverseSurface = NeutralTone20,
        inverseOnSurface = NeutralTone95,
        surfaceTint = BrandTone40,
        scrim = Color.Black,
    )

/** Dark scheme. Accent tones sit light enough to carry dark text. */
val TurboistDarkColors: ColorScheme =
    darkColorScheme(
        primary = BrandTone80,
        onPrimary = BrandTone20,
        primaryContainer = BrandTone30,
        onPrimaryContainer = BrandTone90,
        inversePrimary = BrandTone40,
        secondary = Color(0xFFE7BEAF),
        onSecondary = Color(0xFF442A20),
        secondaryContainer = Color(0xFF5D4035),
        onSecondaryContainer = Color(0xFFFFDBCE),
        tertiary = Color(0xFFD9C68C),
        onTertiary = Color(0xFF3B2F05),
        tertiaryContainer = Color(0xFF534619),
        onTertiaryContainer = Color(0xFFF6E1A6),
        error = Color(0xFFFFB4AB),
        onError = Color(0xFF690005),
        errorContainer = Color(0xFF93000A),
        onErrorContainer = Color(0xFFFFDAD6),
        background = NeutralTone10,
        onBackground = NeutralTone90,
        surface = NeutralTone10,
        onSurface = NeutralTone90,
        surfaceVariant = NeutralVariant30,
        onSurfaceVariant = NeutralVariant80,
        outline = NeutralVariant60,
        outlineVariant = NeutralVariant30,
        inverseSurface = NeutralTone90,
        inverseOnSurface = NeutralTone10,
        surfaceTint = BrandTone80,
        scrim = Color.Black,
    )
