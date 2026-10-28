package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.annotation.StringRes
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.LabelMessage
import ru.tinyops.turboist.nativeapp.projects.ui.projectTint

/** The name of one column of the report — the control the user switches with. */
@StringRes
fun periodLabel(period: LabelUsagePeriod): Int =
    when (period) {
        LabelUsagePeriod.WEEK -> R.string.page_labels_period_week
        LabelUsagePeriod.MONTH -> R.string.page_labels_period_month
        LabelUsagePeriod.QUARTER -> R.string.page_labels_period_quarter
    }

/** How far back the chosen column looks, spelled out under the title. */
@StringRes
fun periodRangeLabel(period: LabelUsagePeriod): Int =
    when (period) {
        LabelUsagePeriod.WEEK -> R.string.page_labels_rangeDays_week
        LabelUsagePeriod.MONTH -> R.string.page_labels_rangeDays_month
        LabelUsagePeriod.QUARTER -> R.string.page_labels_rangeDays_quarter
    }

/**
 * When a label was last reached for, as a sentence.
 *
 * The nearest two days are named rather than counted: "used yesterday" is read
 * at a glance where "used 1 days ago" has to be worked out.
 */
@Composable
fun lastUsedText(daysAgo: Long?): String =
    when (daysAgo) {
        null -> stringResource(R.string.page_labels_lastUsed_never)
        0L -> stringResource(R.string.page_labels_lastUsed_today)
        1L -> stringResource(R.string.page_labels_lastUsed_yesterday)
        // The shared wording carries no type for its values, so every generated
        // placeholder is filled with text rather than with a number.
        else -> stringResource(R.string.page_labels_lastUsed_daysAgo, daysAgo.toString())
    }

/** What a label screen says back, as a sentence. */
@Composable
fun labelMessageText(message: LabelMessage): String =
    when (message) {
        LabelMessage.FAILED -> stringResource(R.string.page_label_failedUpdate)
    }

/** The colour a label is drawn in, or `null` when it has none the app can read. */
fun labelTint(label: Label): Color? = projectTint(label.color)

/**
 * How strongly a chip is filled with its label's colour. Enough to tell labels
 * apart at a glance, little enough that the name on top stays readable in both
 * themes — the same share the web client mixes in.
 */
const val LABEL_FILL_ALPHA = 0.32f

/**
 * A label's colour as a dot, for the places where the name sits on a control
 * of its own and only a mark of the colour fits beside it. A label with no
 * colour gets no dot rather than a grey one: grey would read as a colour too.
 */
@Composable
fun LabelDot(
    label: Label,
    modifier: Modifier = Modifier,
    size: Dp = 10.dp,
) {
    val tint = labelTint(label) ?: return
    Box(modifier = modifier.size(size).background(tint, CircleShape))
}
