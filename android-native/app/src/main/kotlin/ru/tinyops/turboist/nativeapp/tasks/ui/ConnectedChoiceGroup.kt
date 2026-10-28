package ru.tinyops.turboist.nativeapp.tasks.ui

import androidx.compose.animation.core.animateDpAsState
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectableGroup
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp

/** The fill and the content colour an option takes while it is the chosen one. */
data class ChoiceColors(
    val container: Color,
    val content: Color,
)

/**
 * A scale of options as one connected group, exactly one of them chosen.
 *
 * The options sit edge to edge with only their seams cut, so the row reads as a
 * single control; the chosen one rounds out into a pill and gains a tick, which
 * is what tells it apart without colour alone. The seams animate, so changing
 * the choice is something the eye can follow rather than a jump.
 *
 * [equalWidths] splits the row evenly, which suits short codes like P1..P4.
 * Without it each option is as wide as its words and the row scrolls when the
 * words do not fit, which suits labels of very different lengths.
 */
@Composable
internal fun <T> ConnectedChoiceGroup(
    options: List<T>,
    selected: T,
    onSelect: (T) -> Unit,
    label: @Composable (T) -> String,
    modifier: Modifier = Modifier,
    enabled: Boolean = true,
    equalWidths: Boolean = true,
    selectedColors: @Composable (T) -> ChoiceColors = {
        ChoiceColors(MaterialTheme.colorScheme.primaryContainer, MaterialTheme.colorScheme.onPrimaryContainer)
    },
    leading: (@Composable (T) -> Unit)? = null,
) {
    Row(
        modifier =
            modifier
                .fillMaxWidth()
                .then(if (equalWidths) Modifier else Modifier.horizontalScroll(rememberScrollState()))
                .selectableGroup(),
        horizontalArrangement = Arrangement.spacedBy(SEAM),
    ) {
        options.forEachIndexed { index, option ->
            val chosen = option == selected
            val text = label(option)
            val start by animateDpAsState(if (chosen || index == 0) OUTER else INNER, label = "start corner")
            val end by animateDpAsState(
                if (chosen || index == options.lastIndex) OUTER else INNER,
                label = "end corner",
            )
            val colors =
                if (chosen) {
                    selectedColors(option)
                } else {
                    ChoiceColors(MaterialTheme.colorScheme.surfaceContainerHigh, MaterialTheme.colorScheme.onSurface)
                }
            Surface(
                selected = chosen,
                onClick = { onSelect(option) },
                enabled = enabled,
                color = colors.container,
                contentColor = colors.content,
                shape = RoundedCornerShape(topStart = start, bottomStart = start, topEnd = end, bottomEnd = end),
                modifier =
                    Modifier
                        .then(if (equalWidths) Modifier.weight(1f) else Modifier.defaultMinSize(minWidth = 64.dp))
                        .height(HEIGHT)
                        .alpha(if (enabled || chosen) 1f else DISABLED_ALPHA)
                        .semantics {
                            role = Role.RadioButton
                            contentDescription = text
                        },
            ) {
                Row(
                    modifier = Modifier.padding(horizontal = 14.dp),
                    horizontalArrangement = Arrangement.spacedBy(6.dp, Alignment.CenterHorizontally),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    if (chosen) {
                        Icon(Icons.Filled.Check, contentDescription = null, modifier = Modifier.size(16.dp))
                    } else {
                        leading?.invoke(option)
                    }
                    Text(
                        text = text,
                        style = MaterialTheme.typography.labelLarge,
                        fontWeight = if (chosen) FontWeight.Bold else FontWeight.Medium,
                        maxLines = 1,
                        softWrap = false,
                    )
                }
            }
        }
    }
}

/** The height of every option, which is also its hit target. */
private val HEIGHT: Dp = 44.dp

/** The corner on the group's outer edges and on the chosen option: a pill. */
private val OUTER: Dp = 22.dp

/** The corner cut into a seam between two options. */
private val INNER: Dp = 6.dp

/** The gap between two options. */
private val SEAM: Dp = 3.dp

/** How faded an option is while the whole group cannot be changed. */
private const val DISABLED_ALPHA = 0.6f
