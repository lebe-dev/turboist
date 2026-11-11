package ru.tinyops.turboist.nativeapp.switcher

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.quickadd.FakeProjectActivity
import ru.tinyops.turboist.nativeapp.quickadd.ProjectActivity
import ru.tinyops.turboist.nativeapp.quickadd.RecordingQuickAddActions
import ru.tinyops.turboist.nativeapp.tasks.DayClock
import ru.tinyops.turboist.nativeapp.tasks.task
import java.time.Clock
import java.time.LocalDate
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

@OptIn(ExperimentalCoroutinesApi::class)
class QuickSwitcherPresenterTest {
    private val zone = ZoneId.of("Europe/Moscow")
    private val today = LocalDate.of(2026, 11, 4)

    /** 19:30 in Moscow: the evening, by the product's default phases. */
    private val clock = DayClock(Clock.fixed(today.atTime(19, 30).atZone(zone).toInstant(), zone))
    private val actions = RecordingQuickAddActions()

    private fun project(
        id: Long,
        title: String,
        status: ProjectStatus = ProjectStatus.OPEN,
    ) = Project(localId = id, contextLocalId = 1, title = title, status = status, createdAt = 0, updatedAt = 0)

    private val dacha = SwitcherRow.ProjectRow(project(5, "Dacha"))
    private val archived = SwitcherRow.ProjectRow(project(6, "Old", status = ProjectStatus.ARCHIVED))
    private val passport = SwitcherRow.TaskRow(task(1, title = "Renew passport"), projectTitle = null)

    private fun TestScope.presenterWith(
        repository: QuickSwitcherRepository,
        recent: RecentItems,
        activity: ProjectActivity = ProjectActivity.None,
    ) = QuickSwitcherPresenter(
        scope = CoroutineScope(StandardTestDispatcher(testScheduler)),
        repository = repository,
        recentItems = recent,
        actions = actions,
        clock = clock,
        activity = activity,
    )

    @Test
    fun `opening shows the recently opened rows`() =
        runTest {
            val recent = FakeRecentItems(listOf(passport.ref, dacha.ref))
            val presenter = presenterWith(FakeQuickSwitcherRepository(rows = listOf(passport, dacha)), recent)

            presenter.open()
            advanceUntilIdle()

            assertEquals(listOf(passport.ref, dacha.ref), presenter.state.value.rows.map { it.ref })
            assertTrue(presenter.state.value.rows.all { it.recent })
        }

    @Test
    fun `typing searches the replica and keeps the matching recent first`() =
        runTest {
            val other = SwitcherRow.TaskRow(task(2, title = "Passport photo"), projectTitle = null)
            val repository =
                FakeQuickSwitcherRepository(
                    rows = listOf(passport),
                    matches =
                        mapOf(
                            "pass" to SwitcherMatches(tasks = listOf(RankedRow(other, 0), RankedRow(passport, 0))),
                        ),
                )
            val presenter = presenterWith(repository, FakeRecentItems(listOf(passport.ref)))

            presenter.open()
            presenter.type("pass")
            advanceUntilIdle()

            assertEquals(listOf("pass"), repository.searches)
            assertEquals(listOf(passport.ref, other.ref), presenter.state.value.rows.map { it.ref })
        }

    @Test
    fun `for today is offered for the two most recently opened open projects, filed due today in the current phase`() =
        runTest {
            val garden = SwitcherRow.ProjectRow(project(7, "Garden"))
            val third = SwitcherRow.ProjectRow(project(8, "Third"))
            val recent = FakeRecentItems(listOf(archived.ref, passport.ref, dacha.ref, garden.ref, third.ref))
            val presenter =
                presenterWith(
                    FakeQuickSwitcherRepository(rows = listOf(archived, passport, dacha, garden, third)),
                    recent,
                )

            presenter.open()
            presenter.type("  Buy   seeds ")
            advanceUntilIdle()
            assertEquals(listOf("Dacha", "Garden"), presenter.state.value.todayProjects.map { it.title })
            assertEquals(DayPart.EVENING, presenter.state.value.todayPart)

            presenter.create(SwitcherTarget.Today(garden.project))
            advanceUntilIdle()

            val call = actions.calls.single()
            assertEquals(TaskDestination.InProject(7), call.destination)
            assertEquals("Buy seeds", call.task.title)
            assertEquals(today.atStartOfDay(zone).toInstant().toEpochMilli(), call.task.dueAt)
            assertEquals(DayPart.EVENING, call.task.dayPart)
            assertEquals(QuickSwitcherMessage.AddedForToday("Garden"), presenter.messages.first())
            assertFalse(presenter.state.value.visible)
        }

    @Test
    fun `with no recently opened project there is no for today target - the inbox is never one`() =
        runTest {
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Buy seeds")
            advanceUntilIdle()

            assertTrue(presenter.state.value.todayProjects.isEmpty())
        }

    @Test
    fun `to the inbox files an undated task`() =
        runTest {
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Call mom")
            presenter.create(SwitcherTarget.Inbox)
            advanceUntilIdle()

            val call = actions.calls.single()
            assertEquals(TaskDestination.Inbox, call.destination)
            assertNull(call.task.dueAt)
            assertEquals(DayPart.NONE, call.task.dayPart)
            assertEquals(QuickSwitcherMessage.AddedToInbox, presenter.messages.first())
        }

    @Test
    fun `a failed write keeps the panel open with what was typed`() =
        runTest {
            actions.failFrom = 1
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Call mom")
            presenter.create(SwitcherTarget.Inbox)
            advanceUntilIdle()

            assertEquals(QuickSwitcherMessage.Failed, presenter.messages.first())
            assertTrue(presenter.state.value.visible)
            assertEquals("Call mom", presenter.state.value.typed)
            assertFalse(presenter.state.value.creating)
        }

    @Test
    fun `an opened task or project is remembered`() =
        runTest {
            val recent = FakeRecentItems()
            val presenter = presenterWith(FakeQuickSwitcherRepository(), recent)

            presenter.visit(dacha.ref)
            presenter.visit(passport.ref)
            advanceUntilIdle()

            assertEquals(listOf(passport.ref, dacha.ref), recent.remembered)
        }

    @Test
    fun `opening a project, or a task inside one, marks that project as worked with`() =
        runTest {
            val activity = FakeProjectActivity()
            val inGarden =
                SwitcherRow.TaskRow(
                    task(2, title = "Prune roses", projectLocalId = 7),
                    projectTitle = "Garden",
                )
            val presenter =
                presenterWith(
                    FakeQuickSwitcherRepository(rows = listOf(inGarden, passport)),
                    FakeRecentItems(),
                    activity,
                )
            val at = clock.now().toEpochMilli()

            presenter.visit(dacha.ref)
            presenter.visit(inGarden.ref)
            presenter.visit(passport.ref)
            advanceUntilIdle()

            assertEquals(mapOf(5L to at, 7L to at), activity.touched)
        }

    @Test
    fun `adding a task for today marks its project as worked with`() =
        runTest {
            val activity = FakeProjectActivity()
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems(), activity)
            presenter.open()
            presenter.type("Water the beds")
            advanceUntilIdle()

            presenter.create(SwitcherTarget.Today(dacha.project))
            advanceUntilIdle()

            assertEquals(setOf(5L), activity.touched.keys)
        }
}
