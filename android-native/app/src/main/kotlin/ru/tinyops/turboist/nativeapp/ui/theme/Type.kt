package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.material3.Typography
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.LineHeightStyle
import androidx.compose.ui.unit.sp

// Task lists are dense: a screen shows many short one-line rows rather than a
// few paragraphs. The scale below therefore tightens the body and label sizes
// against the Material defaults and keeps the display/headline sizes for the
// handful of places that actually need a heading.
private val Dense =
    LineHeightStyle(
        alignment = LineHeightStyle.Alignment.Center,
        trim = LineHeightStyle.Trim.None,
    )

private fun dense(
    size: Int,
    lineHeight: Int,
    weight: FontWeight = FontWeight.Normal,
    letterSpacing: Double = 0.0,
): TextStyle =
    TextStyle(
        fontFamily = FontFamily.Default,
        fontWeight = weight,
        fontSize = size.sp,
        lineHeight = lineHeight.sp,
        letterSpacing = letterSpacing.sp,
        lineHeightStyle = Dense,
    )

/** The app-wide type scale. Every screen reads its styles from here. */
val TurboistTypography: Typography =
    Typography(
        displaySmall = dense(size = 34, lineHeight = 42, weight = FontWeight.SemiBold),
        headlineLarge = dense(size = 30, lineHeight = 38, weight = FontWeight.SemiBold),
        headlineMedium = dense(size = 26, lineHeight = 34, weight = FontWeight.SemiBold),
        headlineSmall = dense(size = 22, lineHeight = 28, weight = FontWeight.SemiBold),
        titleLarge = dense(size = 20, lineHeight = 26, weight = FontWeight.SemiBold),
        titleMedium = dense(size = 16, lineHeight = 22, weight = FontWeight.Medium, letterSpacing = 0.1),
        titleSmall = dense(size = 14, lineHeight = 20, weight = FontWeight.Medium, letterSpacing = 0.1),
        bodyLarge = dense(size = 16, lineHeight = 22, letterSpacing = 0.15),
        bodyMedium = dense(size = 14, lineHeight = 20, letterSpacing = 0.2),
        bodySmall = dense(size = 12, lineHeight = 16, letterSpacing = 0.3),
        labelLarge = dense(size = 14, lineHeight = 18, weight = FontWeight.Medium, letterSpacing = 0.1),
        labelMedium = dense(size = 12, lineHeight = 16, weight = FontWeight.Medium, letterSpacing = 0.4),
        labelSmall = dense(size = 11, lineHeight = 14, weight = FontWeight.Medium, letterSpacing = 0.4),
    )
