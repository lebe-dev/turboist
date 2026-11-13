package ru.tinyops.turboist.nativeapp.tasks.ui

import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.nativeapp.labels.pickRecentLabels

/** The label sheet's three rows, top to bottom. */
internal data class LabelSheetRows(
    val suggested: List<Label>,
    val recent: List<Label>,
    val rest: List<Label>,
)

/**
 * Lays out the label sheet over [matches], the labels the search leaves.
 *
 * What the model suggests leads while nothing is typed — a search is the user
 * saying what they want instead. Then the labels this device applied lately,
 * then everything else. Each row is lifted out of the rows below it so nothing
 * is offered twice.
 */
internal fun labelSheetRows(
    matches: List<Label>,
    searching: Boolean,
    recentOrder: List<Long>,
    suggestedIds: List<Long>,
): LabelSheetRows {
    val byId = matches.associateBy { it.localId }
    val suggested = if (searching) emptyList() else suggestedIds.mapNotNull { byId[it] }
    val lifted = suggested.mapTo(HashSet()) { it.localId }
    val recent = pickRecentLabels(recentOrder, matches).filterNot { it.localId in lifted }
    recent.mapTo(lifted) { it.localId }
    return LabelSheetRows(suggested, recent, matches.filterNot { it.localId in lifted })
}
