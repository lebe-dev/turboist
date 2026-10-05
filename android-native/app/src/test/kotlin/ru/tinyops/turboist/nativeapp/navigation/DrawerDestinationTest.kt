package ru.tinyops.turboist.nativeapp.navigation

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class DrawerDestinationTest {
    @Test
    fun `covers every top-level destination of the product`() {
        val routes = DrawerDestination.entries.map { it.route }
        assertEquals(
            listOf(
                InboxRoute,
                TodayRoute,
                TomorrowRoute,
                WeekRoute,
                ProjectsRoute,
                LabelsRoute,
                CompletedRoute,
                NextWeekRoute,
                TroikiRoute,
                SearchRoute,
                SettingsRoute,
            ),
            routes,
        )
    }

    @Test
    fun `no destination is listed twice`() {
        val routes = DrawerDestination.entries.map { it.route }
        assertEquals(routes.size, routes.distinct().size)
    }

    @Test
    fun `every entry carries its own label`() {
        val labels = DrawerDestination.entries.map { it.labelRes }
        assertEquals(labels.size, labels.distinct().size)
        assertTrue(labels.none { it == 0 })
    }

    @Test
    fun `grouping keeps declaration order and loses nothing`() {
        val grouped = DrawerDestination.grouped(dailyPlanEnabled = true)

        assertEquals(
            listOf(DrawerGroup.Main, DrawerGroup.Planning, DrawerGroup.More),
            grouped.map { it.first },
        )
        assertEquals(
            DrawerDestination.entries.toList(),
            grouped.flatMap { it.second },
        )
    }

    @Test
    fun `a user who does not keep a daily plan is not offered one`() {
        val offered = DrawerDestination.grouped(dailyPlanEnabled = false).flatMap { it.second }

        assertEquals(
            DrawerDestination.entries - DrawerDestination.Troiki,
            offered,
            "only the plan goes; everything else is a place the product always has",
        )
        assertTrue(
            DrawerDestination.grouped(dailyPlanEnabled = false).any { it.first == DrawerGroup.Planning },
            "the week is still planned, so its heading stays",
        )
    }

    @Test
    fun `drawer holds no destination that needs an argument`() {
        // The drawer is the top level of the app: a project or a label is
        // reached from its list, never from a static menu that would have to
        // invent an id.
        val argumentBearing = listOf(ProjectRoute::class, LabelRoute::class, ContextRoute::class, TaskRoute::class)
        assertTrue(DrawerDestination.entries.none { it.route::class in argumentBearing })
    }
}
