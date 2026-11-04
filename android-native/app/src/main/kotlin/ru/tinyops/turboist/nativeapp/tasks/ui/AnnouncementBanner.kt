package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Campaign
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawWithContent
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.nativeapp.ui.markdown.MarkdownText
import ru.tinyops.turboist.nativeapp.ui.theme.TurboistTheme

/** How long the opening shimmer runs, the web banner's five seconds. */
private const val SHIMMER_MILLIS = 5_000

/** How far into the sweep the outline starts to fade, as the web's keyframe does. */
private const val SHIMMER_FADE_FROM = 0.88f

/** How strong the outline is while it runs. */
internal const val SHIMMER_PEAK_ALPHA = 0.9f

/** How much of the light also washes over the inside, so the whole banner glints. */
private const val SHIMMER_FILL_ALPHA = 0.18f

/** How thick the running outline is: the web's hairline, widened for a phone. */
private val SHIMMER_STROKE = 2.dp

/**
 * Where the centre of the light is, `progress` of the way through the sweep.
 *
 * The web paints a band one banner wide inside a gradient twice as wide and
 * slides it from 250 % to -250 %; that puts the centre one and a half widths off
 * the left edge at the start and as far off the right at the end, so the band
 * enters and leaves the banner whole and crosses it in the middle two fifths.
 */
internal fun shimmerCenter(
    progress: Float,
    width: Float,
): Float = width * (-1.5f + 5f * progress)

/** How strong the outline is `progress` of the way through: steady, then fading to nothing. */
internal fun shimmerAlpha(progress: Float): Float {
    if (progress <= SHIMMER_FADE_FROM) return SHIMMER_PEAK_ALPHA
    val fade = ((progress - SHIMMER_FADE_FROM) / (1f - SHIMMER_FADE_FROM)).coerceIn(0f, 1f)
    return SHIMMER_PEAK_ALPHA * (1f - fade)
}

/**
 * The announcement the user wrote for the day, at the head of the list.
 *
 * It sits on a tint of red with a megaphone beside it so it is seen before the
 * tasks below, and the first time it appears — when the app opens, or when the
 * phase it was scoped to begins — a light runs once along its outline, as on
 * the web. Moving between screens does not replay it: whether it has played is
 * kept with the screen's saved state for as long as the text stays the same,
 * and is forgotten once the announcement goes away, so its next appearance
 * shimmers again. A device set to remove animations finishes the sweep at once
 * and so never shows it.
 */
@Composable
internal fun AnnouncementBanner(
    text: String,
    gutter: Dp,
    corner: Dp,
) {
    val accents = TurboistTheme.accents
    val base = MaterialTheme.colorScheme.surface
    val shape = RoundedCornerShape(corner)

    var played by rememberSaveable(text) { mutableStateOf(false) }
    val progress = remember(text) { Animatable(if (played) 1f else 0f) }
    LaunchedEffect(text) {
        if (played) return@LaunchedEffect
        played = true
        progress.animateTo(1f, tween(durationMillis = SHIMMER_MILLIS, easing = LinearEasing))
    }

    Row(
        verticalAlignment = Alignment.Top,
        modifier =
            Modifier
                .fillMaxWidth()
                .padding(horizontal = gutter, vertical = 4.dp)
                .background(accents.announcementContainer, shape)
                .drawWithContent {
                    drawContent()
                    val p = progress.value
                    if (p >= 1f) return@drawWithContent
                    val w = size.width
                    val center = shimmerCenter(p, w)
                    // The web's 110° slant, kept as a horizontal lead of the band's
                    // top over its bottom.
                    val slant = size.height * 0.36f
                    val light =
                        Brush.linearGradient(
                            colorStops =
                                arrayOf(
                                    0f to Color.Transparent,
                                    0.34f to accents.announcement,
                                    0.5f to base,
                                    0.66f to accents.announcement,
                                    1f to Color.Transparent,
                                ),
                            start = Offset(center - w / 2 - slant, 0f),
                            end = Offset(center + w / 2 + slant, size.height),
                        )
                    val radius = CornerRadius(corner.toPx())
                    val stroke = SHIMMER_STROKE.toPx()
                    val alpha = shimmerAlpha(p)
                    drawRoundRect(brush = light, cornerRadius = radius, alpha = alpha * SHIMMER_FILL_ALPHA)
                    drawRoundRect(
                        brush = light,
                        topLeft = Offset(stroke / 2, stroke / 2),
                        size = Size(w - stroke, size.height - stroke),
                        cornerRadius = CornerRadius(radius.x - stroke / 2),
                        style = Stroke(width = stroke),
                        alpha = alpha,
                    )
                }.semantics { liveRegion = LiveRegionMode.Polite }
                .padding(horizontal = 16.dp, vertical = 12.dp),
    ) {
        Icon(
            imageVector = Icons.Filled.Campaign,
            contentDescription = null,
            tint = accents.announcement,
            modifier = Modifier.padding(end = 12.dp).size(20.dp),
        )
        MarkdownText(
            text = text,
            style = MaterialTheme.typography.bodyMedium.copy(color = MaterialTheme.colorScheme.onSurface),
            modifier = Modifier.weight(1f),
        )
    }
}
