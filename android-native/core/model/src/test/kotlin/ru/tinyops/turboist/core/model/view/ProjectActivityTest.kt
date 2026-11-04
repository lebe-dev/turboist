package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Project
import java.time.Instant
import java.time.ZoneOffset
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** The project radar's counting and layout, kept in step with the web client's. */
class ProjectActivityTest {
    private val now = Instant.parse("2026-07-26T12:00:00Z")
    private val zone = ZoneOffset.UTC
    private val day = 86_400_000L

    private fun project(
        localId: Long,
        title: String,
    ) = Project(localId = localId, contextLocalId = 1, title = title, createdAt = 0, updatedAt = 0)

    private fun daysAgo(days: Int) = now.toEpochMilli() - days * day

    private fun activity(
        title: String,
        week: ProjectPeriodActivity = ProjectPeriodActivity(),
    ) = ProjectActivity(project(title.hashCode().toLong(), title), mapOf(LabelUsagePeriod.WEEK to week))

    @Test
    fun `completions are bucketed into the window and the one before it`() {
        val rows =
            projectActivity(
                listOf(project(1, "a"), project(2, "quiet")),
                listOf(
                    ProjectCompletion(1, daysAgo(1)),
                    ProjectCompletion(1, daysAgo(3)),
                    ProjectCompletion(1, daysAgo(10)),
                    ProjectCompletion(1, daysAgo(45)),
                ),
                now,
                zone,
            )
        val a = rows.single { it.project.title == "a" }
        assertEquals(ProjectPeriodActivity(completed = 2, previousCompleted = 1), a.period(LabelUsagePeriod.WEEK))
        assertEquals(ProjectPeriodActivity(completed = 3, previousCompleted = 1), a.period(LabelUsagePeriod.MONTH))
        assertEquals(ProjectPeriodActivity(), rows.single { it.project.title == "quiet" }.period(LabelUsagePeriod.WEEK))
    }

    @Test
    fun `three projects with activity are needed to make a shape`() {
        val rows =
            listOf(
                activity("a", ProjectPeriodActivity(3)),
                activity("b", ProjectPeriodActivity(1)),
                activity("c"),
            )
        assertNull(projectRadar(rows, LabelUsagePeriod.WEEK))
    }

    @Test
    fun `spokes follow the title and scale to the busiest project in either window`() {
        val radar =
            projectRadar(
                listOf(
                    activity("delta", ProjectPeriodActivity(4, 1)),
                    activity("alpha", ProjectPeriodActivity(2, 0)),
                    activity("gone", ProjectPeriodActivity(0, 8)),
                ),
                LabelUsagePeriod.WEEK,
            )!!
        assertEquals(listOf("alpha", "delta", "gone"), radar.axes.map { it.activity.project.title })
        assertEquals(8, radar.scaleMax)
        assertEquals(0f, radar.axes.single { it.activity.project.title == "gone" }.current)
        assertEquals(1f, radar.axes.single { it.activity.project.title == "gone" }.previous)
    }

    @Test
    fun `past the cap the busiest projects win and the rest are counted as hidden`() {
        val radar =
            projectRadar(
                listOf(
                    activity("a", ProjectPeriodActivity(9)),
                    activity("b", ProjectPeriodActivity(5)),
                    activity("c", ProjectPeriodActivity(3)),
                    activity("d", ProjectPeriodActivity(1)),
                ),
                LabelUsagePeriod.WEEK,
                maxAxes = 3,
            )!!
        assertEquals(listOf("a", "b", "c"), radar.axes.map { it.activity.project.title })
        assertEquals(1, radar.hidden)
    }
}
