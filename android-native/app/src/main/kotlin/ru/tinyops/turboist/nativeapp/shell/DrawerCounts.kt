package ru.tinyops.turboist.nativeapp.shell

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import ru.tinyops.turboist.nativeapp.navigation.DrawerDestination
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The numbers the drawer paints beside its entries.
 *
 * Every field is nullable and null means "not known yet" rather than zero, so a
 * drawer opened before the replica has anything to say shows no badge at all
 * instead of a row of confident zeroes.
 */
data class DrawerCounts(
    val inbox: Int? = null,
    val weekPlanned: Int? = null,
    val weekLimit: Int? = null,
    /**
     * How many changes the server refused and nobody has cleared yet.
     *
     * It is a drawer counter rather than part of the sync strip because it is
     * not news: the refusal already happened, the strip about it has long gone,
     * and what is left is a pile waiting for a person. A mark on the way to it
     * is how that pile stays findable without interrupting anything.
     */
    val unsentChanges: Int? = null,
)

/** What a single drawer entry shows on its trailing edge, if anything. */
sealed interface DrawerBadge {
    /** A bare number, e.g. how many tasks sit in the inbox. */
    data class Count(val value: Int) : DrawerBadge

    /** Usage against a cap, e.g. how much of the weekly plan is spent. */
    data class Ratio(val current: Int, val limit: Int) : DrawerBadge
}

/**
 * The badge for one destination, or null when there is nothing worth showing.
 *
 * An inbox count of zero is deliberately silent: an empty inbox is the desired
 * state and does not need a marker, and so is a settings entry with nothing set
 * aside behind it. A plan ratio is shown as soon as both the
 * planned count and the cap are known, including at zero, because "0/12" is the
 * useful reading of an untouched week.
 */
fun DrawerCounts.badgeFor(destination: DrawerDestination): DrawerBadge? =
    when (destination) {
        DrawerDestination.Inbox -> inbox?.takeIf { it > 0 }?.let(DrawerBadge::Count)
        DrawerDestination.Settings -> unsentChanges?.takeIf { it > 0 }?.let(DrawerBadge::Count)
        DrawerDestination.Week, DrawerDestination.NextWeek -> {
            val planned = weekPlanned
            val limit = weekLimit
            if (planned != null && limit != null) DrawerBadge.Ratio(planned, limit) else null
        }

        else -> null
    }

/**
 * Where the drawer's counters come from.
 *
 * A seam, exactly like the session one: today a stand-in publishes nothing, and
 * the implementation that observes the on-device replica replaces the binding
 * without the drawer changing.
 */
interface DrawerCountsSource {
    val counts: StateFlow<DrawerCounts>
}

/**
 * Stand-in [DrawerCountsSource] used until the replica can be queried. It
 * publishes an empty [DrawerCounts] forever, which renders a drawer with no
 * badges rather than one with invented numbers.
 */
@Singleton
class EmptyDrawerCountsSource
    @Inject
    constructor() : DrawerCountsSource {
        override val counts: StateFlow<DrawerCounts> = MutableStateFlow(DrawerCounts()).asStateFlow()
    }
