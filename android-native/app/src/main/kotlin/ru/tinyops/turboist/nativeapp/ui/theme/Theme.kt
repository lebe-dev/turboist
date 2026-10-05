package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider

/**
 * Wraps the whole content tree in the app's theme.
 *
 * The palette is the product's own in every case — the wallpaper-derived scheme
 * Android offers is deliberately not taken. A system-themed app feels native,
 * but Turboist is a client for a workspace the user also opens in a browser, and
 * a phone whose wallpaper is a grey photograph turns the same lists that are
 * warm and red on a laptop into a grey-on-grey page. The colours here are the
 * web client's, so the two front ends are recognisably one product on any
 * device; the only thing the phone still decides is [darkTheme].
 *
 * The signalling colours travel alongside the scheme rather than inside it: see
 * [TurboistAccents].
 */
@Composable
fun TurboistTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    content: @Composable () -> Unit,
) {
    CompositionLocalProvider(
        LocalTurboistAccents provides if (darkTheme) TurboistDarkAccents else TurboistLightAccents,
    ) {
        MaterialTheme(
            colorScheme = if (darkTheme) TurboistDarkColors else TurboistLightColors,
            typography = TurboistTypography,
            content = content,
        )
    }
}
