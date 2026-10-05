package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.material3.ColorScheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.ui.graphics.Color

/**
 * The product's brand orange: the colour of the launcher icon and of the browser
 * chrome the web client asks for. It is not the accent the screens are drawn
 * with — that is [WebPrimary] below — but it is what the product looks like from
 * the outside, so it stays named and stays exact.
 */
val BrandOrange: Color = Color(0xFFE2580E)

/*
 * The colours below are the web client's own, taken from the custom properties
 * in `frontend/src/routes/layout.css` and converted from OKLCH to sRGB. Turboist
 * is one product with two front ends, and a user who keeps the web app open on a
 * laptop and this one in a pocket must not feel they are looking at two
 * different things.
 *
 * They are transcribed rather than generated from a seed on purpose. Material's
 * tonal-palette generator produces a self-consistent set of its own, but not
 * this set: the web client's neutrals are warm — every grey is pulled a little
 * towards the accent's hue, so nothing beside it reads as blue — and its accent
 * is a red that a generator asked for the brand orange would never land on.
 *
 * Each value is commented with the custom property it comes from. A role
 * Material needs and the web has no token for is derived here and says so.
 */

// --- Light scheme, from `:root`. ---
private val WebBackground = Color(0xFFFBFAF7) // --background
private val WebForeground = Color(0xFF16100C) // --foreground
private val WebCard = Color(0xFFFFFFFF) // --card
private val WebPrimary = Color(0xFFEE343B) // --primary
private val WebPrimaryForeground = Color(0xFFFAFAFA) // --primary-foreground
private val WebSecondary = Color(0xFFF4F1ED) // --secondary
private val WebMuted = Color(0xFFF3EFEC) // --muted
private val WebMutedForeground = Color(0xFF69625D) // --muted-foreground
private val WebAccent = Color(0xFFF2EEE9) // --accent
private val WebAccentForeground = Color(0xFF1F1915) // --accent-foreground
private val WebDestructive = Color(0xFFEA1F2F) // --destructive
private val WebBorder = Color(0xFFE4E1DD) // --border
private val WebSidebar = Color(0xFFF7F5F1) // --sidebar
private val WebSidebarAccent = Color(0xFFEBE7E2) // --sidebar-accent

// Roles the web has no token for, kept on the same warm neutral ramp: a filled
// secondary, a mid outline (the web's --border is far too pale to draw a control
// with), and the surface a snackbar inverts onto.
private val WebSecondaryFill = Color(0xFF7B6F66)
private val WebOutline = Color(0xFF8C857F)
private val WebSurfaceDim = Color(0xFFE8E4E0)
private val WebInverseSurface = Color(0xFF25211D)
private val WebInverseOnSurface = Color(0xFFF4F1EE)

// The accent's container pair, and the brand orange as Material's third accent —
// the one role in the scheme where the product's own colour still shows up.
private val WebPrimaryContainer = Color(0xFFFFD9D4)
private val WebOnPrimaryContainer = Color(0xFF65000A)
private val BrandDeep = Color(0xFFC24700)
private val BrandContainer = Color(0xFFFFDBBF)
private val BrandOnContainer = Color(0xFF541600)

// --- Dark scheme, from `.dark`. ---
private val WebDarkBackground = Color(0xFF100E0C) // --background
private val WebDarkForeground = Color(0xFFF3F1EE) // --foreground
private val WebDarkCard = Color(0xFF191714) // --card
private val WebDarkPopover = Color(0xFF1A1816) // --popover
private val WebDarkPrimary = Color(0xFFFC4447) // --primary
private val WebDarkPrimaryForeground = Color(0xFFFCFCFC) // --primary-foreground
private val WebDarkSecondary = Color(0xFF252220) // --secondary
private val WebDarkMuted = Color(0xFF23201E) // --muted
private val WebDarkMutedForeground = Color(0xFF9C9793) // --muted-foreground
private val WebDarkAccent = Color(0xFF292624) // --accent
private val WebDarkDestructive = Color(0xFFFF5957) // --destructive
private val WebDarkSidebar = Color(0xFF0A0806) // --sidebar

// The same derived roles, one ramp darker. The dark scheme's borders are the one
// place the transcription cannot be literal: the web states them as white at 8%,
// which only means something over a known backdrop, so they are flattened here
// against the surface they sit on.
private val WebDarkSecondaryFill = Color(0xFFADA397)
private val WebDarkOnSecondaryFill = Color(0xFF231E1B)
private val WebDarkOutline = Color(0xFF77706B)
private val WebDarkOutlineVariant = Color(0xFF36322F)
private val WebDarkPrimaryContainer = Color(0xFF79191B)
private val WebDarkOnPrimaryContainer = Color(0xFFFFCFCA)
private val BrandLight = Color(0xFFF7A062)
private val BrandOnLight = Color(0xFF450F00)
private val BrandDarkContainer = Color(0xFF762F06)
private val BrandOnDarkContainer = Color(0xFFFDD5B7)
private val WebDarkOnError = Color(0xFF3D0001)

/** The light scheme, as the web client draws itself in daylight. */
val TurboistLightColors: ColorScheme =
    lightColorScheme(
        primary = WebPrimary,
        onPrimary = WebPrimaryForeground,
        primaryContainer = WebPrimaryContainer,
        onPrimaryContainer = WebOnPrimaryContainer,
        inversePrimary = WebDarkPrimary,
        secondary = WebSecondaryFill,
        onSecondary = Color.White,
        // What the web fills a picked row with, which is exactly what Material
        // reaches for secondaryContainer to do.
        secondaryContainer = WebAccent,
        onSecondaryContainer = WebAccentForeground,
        tertiary = BrandDeep,
        onTertiary = Color.White,
        tertiaryContainer = BrandContainer,
        onTertiaryContainer = BrandOnContainer,
        // The web draws a destructive action in the same red as a primary one,
        // and so does this. Splitting them here would be a second opinion about
        // the product's own wording of "careful now".
        error = WebDestructive,
        onError = Color.White,
        errorContainer = WebPrimaryContainer,
        onErrorContainer = WebOnPrimaryContainer,
        background = WebBackground,
        onBackground = WebForeground,
        surface = WebBackground,
        onSurface = WebForeground,
        surfaceVariant = WebMuted,
        onSurfaceVariant = WebMutedForeground,
        surfaceTint = WebPrimary,
        inverseSurface = WebInverseSurface,
        inverseOnSurface = WebInverseOnSurface,
        outline = WebOutline,
        outlineVariant = WebBorder,
        scrim = Color.Black,
        surfaceBright = WebCard,
        surfaceDim = WebSurfaceDim,
        // The elevation ramp is the web's own layering read from the page
        // outwards: a card, then the sidebar, then the two fills it uses to lift
        // something off it.
        surfaceContainerLowest = WebCard,
        surfaceContainerLow = WebSidebar,
        surfaceContainer = WebSecondary,
        surfaceContainerHigh = WebAccent,
        surfaceContainerHighest = WebSidebarAccent,
    )

/** The dark scheme, from the same stylesheet's `.dark` block. */
val TurboistDarkColors: ColorScheme =
    darkColorScheme(
        primary = WebDarkPrimary,
        onPrimary = WebDarkPrimaryForeground,
        primaryContainer = WebDarkPrimaryContainer,
        onPrimaryContainer = WebDarkOnPrimaryContainer,
        inversePrimary = WebPrimary,
        secondary = WebDarkSecondaryFill,
        onSecondary = WebDarkOnSecondaryFill,
        secondaryContainer = WebDarkAccent,
        onSecondaryContainer = WebDarkForeground,
        tertiary = BrandLight,
        onTertiary = BrandOnLight,
        tertiaryContainer = BrandDarkContainer,
        onTertiaryContainer = BrandOnDarkContainer,
        error = WebDarkDestructive,
        onError = WebDarkOnError,
        errorContainer = WebDarkPrimaryContainer,
        onErrorContainer = WebDarkOnPrimaryContainer,
        background = WebDarkBackground,
        onBackground = WebDarkForeground,
        surface = WebDarkBackground,
        onSurface = WebDarkForeground,
        surfaceVariant = WebDarkMuted,
        onSurfaceVariant = WebDarkMutedForeground,
        surfaceTint = WebDarkPrimary,
        inverseSurface = WebDarkForeground,
        inverseOnSurface = WebDarkBackground,
        outline = WebDarkOutline,
        outlineVariant = WebDarkOutlineVariant,
        scrim = Color.Black,
        surfaceBright = WebDarkOutlineVariant,
        surfaceDim = WebDarkSidebar,
        surfaceContainerLowest = WebDarkSidebar,
        surfaceContainerLow = WebDarkCard,
        surfaceContainer = WebDarkPopover,
        surfaceContainerHigh = WebDarkSecondary,
        surfaceContainerHighest = WebDarkAccent,
    )
