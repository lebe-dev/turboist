package ru.tinyops.turboist.core.model.view

import kotlin.math.PI

/** The radar never draws more spokes than this: past a dozen the names collide. */
const val LABEL_RADAR_MAX_AXES = 12

/** Fewer spokes than this make a line or a point, not a shape. */
const val LABEL_RADAR_MIN_AXES = 3

/**
 * One spoke of the label radar.
 *
 * [applied] is the chosen window's count. [current] and [previous] are fractions
 * of the chart's radius (0..1) for the chosen window and the one before it;
 * [angle] is in radians, clockwise from twelve o'clock.
 */
data class LabelRadarAxis(
    val usage: LabelUsage,
    val applied: Int,
    val angle: Double,
    val current: Float,
    val previous: Float,
)

/**
 * The labels laid out as directions the work pulls in.
 *
 * [scaleMax] is the application count the outer ring stands for; [hidden] is how
 * many labels with activity were left without a spoke because of the cap.
 */
data class LabelRadar(
    val axes: List<LabelRadarAxis>,
    val scaleMax: Int,
    val hidden: Int,
)

/**
 * Lays the report out as a radar: every label a direction, the chosen window a
 * filled shape, the window before it an outline to compare against.
 *
 * A label earns a spoke when it was applied in either window, so one that stopped
 * pulling still shows where the pull used to be. Past [maxAxes] the busiest
 * labels win. Spokes are then placed by name rather than by count, so a label
 * keeps its direction when the period changes and only the shape moves. Null
 * when there are too few spokes to make a shape. The web client lays it out the
 * same way.
 */
fun labelRadar(
    rows: List<LabelUsage>,
    period: LabelUsagePeriod,
    maxAxes: Int = LABEL_RADAR_MAX_AXES,
): LabelRadar? {
    fun weight(row: LabelUsage) = maxOf(row.period(period).applied, row.period(period).previousApplied)
    val candidates =
        rows
            .filter { weight(it) > 0 }
            .sortedWith(compareByDescending<LabelUsage> { weight(it) }.thenBy { it.label.name.lowercase() })
    val picked =
        candidates
            .take(maxAxes)
            .sortedWith(compareBy<LabelUsage> { it.label.name.lowercase() }.thenBy { it.label.localId })
    if (picked.size < LABEL_RADAR_MIN_AXES) return null
    val scaleMax = maxOf(1, picked.maxOf { weight(it) })
    return LabelRadar(
        axes =
            picked.mapIndexed { index, row ->
                LabelRadarAxis(
                    usage = row,
                    applied = row.period(period).applied,
                    angle = 2 * PI * index / picked.size,
                    current = row.period(period).applied.toFloat() / scaleMax,
                    previous = row.period(period).previousApplied.toFloat() / scaleMax,
                )
            },
        scaleMax = scaleMax,
        hidden = candidates.size - picked.size,
    )
}
