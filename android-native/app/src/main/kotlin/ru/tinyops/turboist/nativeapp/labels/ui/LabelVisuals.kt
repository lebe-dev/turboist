package ru.tinyops.turboist.nativeapp.labels.ui

import androidx.annotation.StringRes
import androidx.compose.runtime.Composable
import androidx.compose.ui.res.stringResource
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.nativeapp.R
import ru.tinyops.turboist.nativeapp.labels.LabelMessage

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
