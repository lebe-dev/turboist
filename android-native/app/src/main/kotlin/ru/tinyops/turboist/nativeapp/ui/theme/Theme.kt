package ru.tinyops.turboist.nativeapp.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.ColorScheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.platform.LocalContext

/**
 * Wraps the whole content tree in the app's Material 3 theme.
 *
 * On Android 12 and newer the user's wallpaper colours win by default, because
 * a system-themed app feels native; every other device falls back to the brand
 * palette. Passing [dynamicColor] = false forces the brand palette everywhere,
 * which is what previews and screenshot tests want so their output does not
 * depend on the host device's wallpaper.
 */
@Composable
fun TurboistTheme(
    darkTheme: Boolean = isSystemInDarkTheme(),
    dynamicColor: Boolean = true,
    content: @Composable () -> Unit,
) {
    val context = LocalContext.current
    val colors: ColorScheme =
        when {
            dynamicColor && Build.VERSION.SDK_INT >= Build.VERSION_CODES.S ->
                if (darkTheme) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)

            darkTheme -> TurboistDarkColors
            else -> TurboistLightColors
        }

    MaterialTheme(
        colorScheme = colors,
        typography = TurboistTypography,
        content = content,
    )
}
