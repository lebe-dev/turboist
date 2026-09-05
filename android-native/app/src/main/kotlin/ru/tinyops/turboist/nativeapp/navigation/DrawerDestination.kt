package ru.tinyops.turboist.nativeapp.navigation

import androidx.annotation.StringRes
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.Label
import androidx.compose.material.icons.outlined.CalendarMonth
import androidx.compose.material.icons.outlined.CheckCircle
import androidx.compose.material.icons.outlined.DateRange
import androidx.compose.material.icons.outlined.Event
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.Inbox
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Today
import androidx.compose.material.icons.outlined.ViewColumn
import androidx.compose.ui.graphics.vector.ImageVector
import ru.tinyops.turboist.nativeapp.R

/** The headings the drawer groups its entries under. */
enum class DrawerGroup(
    @param:StringRes val titleRes: Int,
) {
    Main(R.string.nav_main),
    Planning(R.string.nav_planning),
    More(R.string.native_nav_group_more),
}

/**
 * The navigation drawer's catalog, in the order it is drawn.
 *
 * Only argument-free destinations belong here: the drawer is the top level, and
 * a detail screen is reached from a list, never from a static menu. Adding an
 * entry is enough to make a destination appear — the drawer never carries a
 * hand-maintained second copy of this list.
 */
enum class DrawerDestination(
    val route: Any,
    @param:StringRes val labelRes: Int,
    val icon: ImageVector,
    val group: DrawerGroup,
) {
    Inbox(InboxRoute, R.string.nav_inbox, Icons.Outlined.Inbox, DrawerGroup.Main),
    Today(TodayRoute, R.string.nav_today, Icons.Outlined.Today, DrawerGroup.Main),
    Tomorrow(TomorrowRoute, R.string.nav_tomorrow, Icons.Outlined.Event, DrawerGroup.Main),
    Week(WeekRoute, R.string.nav_week, Icons.Outlined.CalendarMonth, DrawerGroup.Main),
    Projects(ProjectsRoute, R.string.nav_projects, Icons.Outlined.Folder, DrawerGroup.Main),
    Labels(LabelsRoute, R.string.nav_labels, Icons.AutoMirrored.Outlined.Label, DrawerGroup.Main),
    Completed(CompletedRoute, R.string.nav_completed, Icons.Outlined.CheckCircle, DrawerGroup.Main),

    NextWeek(NextWeekRoute, R.string.nav_nextWeek, Icons.Outlined.DateRange, DrawerGroup.Planning),
    Troiki(TroikiRoute, R.string.nav_troiki, Icons.Outlined.ViewColumn, DrawerGroup.Planning),

    Search(SearchRoute, R.string.nav_search, Icons.Outlined.Search, DrawerGroup.More),
    Settings(SettingsRoute, R.string.nav_settings, Icons.Outlined.Settings, DrawerGroup.More),
    ;

    companion object {
        /**
         * The catalog grouped for rendering, groups kept in declaration order.
         *
         * [dailyPlanEnabled] is the user's own preference for the daily plan.
         * Turned off, the plan is not part of this installation's product at all
         * and its entry is left out rather than shown leading to an empty screen.
         * It is read from the replicated preference document, so a device that
         * has not synced yet answers "off" and starts offering the plan the
         * moment the preference arrives — the same way every other replicated
         * setting behaves on a fresh install.
         */
        fun grouped(dailyPlanEnabled: Boolean): List<Pair<DrawerGroup, List<DrawerDestination>>> =
            DrawerGroup.entries
                .map { group ->
                    group to entries.filter { it.group == group && it.isOffered(dailyPlanEnabled) }
                }.filter { (_, items) -> items.isNotEmpty() }

        private fun DrawerDestination.isOffered(dailyPlanEnabled: Boolean): Boolean = this != Troiki || dailyPlanEnabled
    }
}
