package ru.tinyops.turboist.nativeapp.shell

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import ru.tinyops.turboist.core.database.dao.TaskViewDao
import ru.tinyops.turboist.core.model.view.ViewWindows
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import ru.tinyops.turboist.nativeapp.navigation.DrawerDestination
import ru.tinyops.turboist.nativeapp.tasks.DayClock
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
    val weekDone: Int? = null,
    val weekTotal: Int? = null,
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

    /** Finished work out of all of it, e.g. how far through the week the user is. */
    data class Progress(val done: Int, val total: Int) : DrawerBadge
}

/**
 * The badge for one destination, or null when there is nothing worth showing.
 *
 * An inbox count of zero is deliberately silent: an empty inbox is the desired
 * state and does not need a marker, and so is a settings entry with nothing set
 * aside behind it. Week progress is shown once the week has anything in it,
 * including at no work done, because "0/12" is the useful reading of an untouched
 * week while "0/0" says nothing at all.
 */
fun DrawerCounts.badgeFor(destination: DrawerDestination): DrawerBadge? =
    when (destination) {
        DrawerDestination.Inbox -> inbox?.takeIf { it > 0 }?.let(DrawerBadge::Count)
        DrawerDestination.Settings -> unsentChanges?.takeIf { it > 0 }?.let(DrawerBadge::Count)
        DrawerDestination.Week -> {
            val done = weekDone
            val total = weekTotal
            if (done != null && total != null && total > 0) DrawerBadge.Progress(done, total) else null
        }

        else -> null
    }

/** Where the drawer's counters come from. */
interface DrawerCountsSource {
    val counts: StateFlow<DrawerCounts>
}

/**
 * The counters read off the replica.
 *
 * The query is a standing one, so completing a task anywhere moves the number
 * without the drawer asking; the clock is only there to move on to the next week
 * when Monday comes, which no change to the replica would announce.
 */
@Singleton
class ReplicaDrawerCountsSource
    @Inject
    constructor(
        views: TaskViewDao,
        clock: DayClock,
        @param:ApplicationScope scope: CoroutineScope,
    ) : DrawerCountsSource {
        @OptIn(ExperimentalCoroutinesApi::class)
        override val counts: StateFlow<DrawerCounts> =
            clock.ticks()
                .map { ViewWindows.week(it, clock.zone) }
                .distinctUntilChanged()
                .flatMapLatest { views.observeWeekProgress(it.from, it.untilExclusive) }
                .map { DrawerCounts(weekDone = it.done, weekTotal = it.total) }
                .stateIn(scope, SharingStarted.WhileSubscribed(), DrawerCounts())
    }
