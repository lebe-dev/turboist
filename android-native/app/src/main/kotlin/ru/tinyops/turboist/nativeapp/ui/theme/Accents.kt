package ru.tinyops.turboist.nativeapp.ui.theme

import androidx.compose.runtime.Composable
import androidx.compose.runtime.Immutable
import androidx.compose.runtime.ReadOnlyComposable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.graphics.Color

/**
 * The colours that stand for something rather than filling a role in the design.
 *
 * A scheme's roles say how prominent a thing is; these say what it *is*. How
 * urgent a task is, that it comes back every week, that it has not reached the
 * server yet — the web client signals each of those with a fixed colour picked
 * for the meaning, not with its accent, and a user reads them the same way on
 * both clients or the colour is not carrying anything.
 *
 * They are therefore kept out of [TurboistLightColors]: putting them in would
 * make them roles, and a role is something a screen may reasonably re-use for a
 * different purpose. The values are the web client's Tailwind steps, converted
 * from OKLCH to sRGB, with the two the web restates for dark mode restated here
 * as well — a green dark enough to read on paper is too dark to read at night.
 *
 * @property priorityHigh what P1 is marked in.
 * @property priorityMedium what P2 is marked in.
 * @property priorityLow what P3 is marked in. P4 is deliberately absent: a task
 *   with no priority carries no signal, and takes the outline colour any plain
 *   control would.
 * @property demanding the mark on a task the user has called complex. The same
 *   red as [priorityHigh] and separate from it on purpose — one is a level the
 *   user chose, the other a warning about the shape of the work.
 * @property recurring the mark on a task that comes back.
 * @property pending text saying a write is still queued on this device.
 * @property pendingContainer what that text sits on: the same amber at the
 *   sliver of opacity the web uses, so it reads as a tint of the page rather
 *   than as a second surface.
 */
@Immutable
data class TurboistAccents(
    val priorityHigh: Color,
    val priorityMedium: Color,
    val priorityLow: Color,
    val demanding: Color,
    val recurring: Color,
    val pending: Color,
    val pendingContainer: Color,
)

// The Tailwind steps the web names, by their own names, so a change over there
// can be followed here without decoding a hex value first.
private val Red500 = Color(0xFFFB2C36)
private val Amber500 = Color(0xFFFE9A00)
private val Amber700 = Color(0xFFBB4D00)
private val Amber300 = Color(0xFFFFD230)
private val Blue500 = Color(0xFF2B7FFF)
private val Emerald600 = Color(0xFF009966)
private val Emerald400 = Color(0xFF00D492)

/** How much of the amber the queued-write badge's backdrop keeps. The web's `bg-amber-500/15`. */
private const val PENDING_TINT_ALPHA = 0.15f

/** The signals as the web client draws them in daylight. */
val TurboistLightAccents: TurboistAccents =
    TurboistAccents(
        priorityHigh = Red500,
        priorityMedium = Amber500,
        priorityLow = Blue500,
        demanding = Red500,
        recurring = Emerald600,
        pending = Amber700,
        pendingContainer = Amber500.copy(alpha = PENDING_TINT_ALPHA),
    )

/**
 * The same signals at night.
 *
 * The three priorities keep their daylight values — they are drawn as a ring or
 * a glyph rather than as text, and they carry on a dark page as they are — while
 * the two that are read as words move a step lighter, exactly as the web's
 * `dark:` variants do.
 */
val TurboistDarkAccents: TurboistAccents =
    TurboistLightAccents.copy(
        recurring = Emerald400,
        pending = Amber300,
    )

/** The signals in force for the tree below the theme. */
val LocalTurboistAccents = staticCompositionLocalOf { TurboistLightAccents }

/** Reaches the app's own colours the way `MaterialTheme` reaches the scheme's. */
object TurboistTheme {
    val accents: TurboistAccents
        @Composable
        @ReadOnlyComposable
        get() = LocalTurboistAccents.current
}
