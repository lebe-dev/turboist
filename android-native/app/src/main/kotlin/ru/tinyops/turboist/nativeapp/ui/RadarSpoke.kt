package ru.tinyops.turboist.nativeapp.ui

/**
 * One spoke of a radar, whatever it stands for: a label, a project.
 *
 * [current] and [previous] are fractions of the chart's radius (0..1) for the
 * chosen window and the one before it; [angle] is in radians, clockwise from
 * twelve o'clock. [colorToken] is the colour the dot wears, in the same form a
 * project or label colour is stored.
 */
data class RadarSpoke(
    val name: String,
    val count: Int,
    val colorToken: String,
    val angle: Double,
    val current: Float,
    val previous: Float,
)
