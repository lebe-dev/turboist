package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Priority
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.sync.write.TaskDestination
import java.time.Instant
import java.time.LocalDate
import java.time.LocalDateTime
import java.time.LocalTime
import java.time.ZoneId
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/**
 * How capture behaves, with nothing behind it.
 *
 * Every case here is about a decision the surface makes — where the task goes,
 * what travels with it, what happens when only half of it is written down — and
 * none of them about the write path, which has its own checks. That separation
 * is why the whole of it can be driven without a database, a clock or a phone.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class QuickAddPresenterTest {
    private val actions = RecordingQuickAddActions()
    private val recent = FakeRecentProjects()
    private val recentLabels = FakeRecentLabels()
    private val zone: ZoneId = ZoneId.of("Europe/Berlin")

    private val workspace =
        MutableStateFlow(
            QuickAddWorkspace(
                projects =
                    listOf(
                        project(1, "Home"),
                        project(2, "Work"),
                        project(3, "Garden"),
                        project(4, "Music"),
                        project(5, "Books"),
                        project(6, "Motor"),
                    ),
                labels = listOf(label(1, "errand"), label(2, "urgent")),
            ),
        )

    /**
     * A presenter on a scope of the test's own, sharing the test's clock.
     *
     * Not the test scope itself: it watches the workspace for as long as it
     * lives, and a watcher that never finishes would keep a test that owns it
     * from finishing either.
     */
    private fun TestScope.presenter(
        workspace: Flow<QuickAddWorkspace> = this@QuickAddPresenterTest.workspace,
        recent: RecentProjects = this@QuickAddPresenterTest.recent,
        now: Instant = NOW,
    ): QuickAddPresenter =
        QuickAddPresenter(
            scope = CoroutineScope(StandardTestDispatcher(testScheduler)),
            workspace = workspace,
            recent = recent,
            actions = actions,
            zone = zone,
            recentLabels = recentLabels,
            now = { now },
        )

    /**
     * Watches the state for the duration of the case.
     *
     * The state is shared only while something is looking at it, so a case that
     * never looks would be asserting against the value it started with.
     */
    private fun TestScope.watch(presenter: QuickAddPresenter) {
        backgroundScope.launch { presenter.state.collect { } }
        runCurrent()
    }

    @Test
    fun `an empty draft cannot be saved, and saving it writes nothing`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("   \n  ")
            runCurrent()

            assertFalse(presenter.state.value.draft.canSubmit)
            presenter.submit()
            runCurrent()
            assertEquals(emptyList(), actions.calls)
        }

    @Test
    fun `a captured thought goes to the inbox by default`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Renew the passport")
            presenter.submit()
            runCurrent()

            assertEquals(1, actions.calls.size)
            assertEquals(TaskDestination.Inbox, actions.calls.single().destination)
            assertEquals("Renew the passport", actions.calls.single().task.title)
            assertFalse(presenter.state.value.visible, "the sheet stayed open after saving")
        }

    @Test
    fun `one line is one task, and they all land in the same place`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.chooseProject(2)
            presenter.setTitles("Buy milk\n\nCall the dentist\nBook the flights")
            presenter.setPriority(Priority.HIGH)
            presenter.submit()
            runCurrent()

            assertEquals(
                listOf("Buy milk", "Call the dentist", "Book the flights"),
                actions.calls.map { it.task.title },
            )
            assertTrue(actions.calls.all { it.destination == TaskDestination.InProject(2) })
            assertTrue(actions.calls.all { it.task.priority == Priority.HIGH })
        }

    @Test
    fun `filing into a project is remembered for the next capture`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.chooseProject(3)
            presenter.setTitles("Water the tomatoes")
            presenter.submit()
            runCurrent()

            assertEquals(listOf(3L), recent.remembered)
        }

    @Test
    fun `labels chosen by hand are remembered once the task is saved, and lead the picker next time`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings = AppSettings(autoLabels = listOf(AutoLabelRule("milk", listOf(1), ignoreCase = true))),
                )
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Buy milk")
            presenter.setLabels(listOf("urgent"))
            runCurrent()
            assertEquals(emptyList(), recentLabels.remembered, "nothing is remembered before the save")

            presenter.submit()
            runCurrent()

            // The label the title earned was not a choice, so it is not a habit either.
            assertEquals(listOf(2L), recentLabels.remembered)
            assertEquals(listOf(2L), presenter.state.value.recentLabelOrder)
        }

    @Test
    fun `filing into the inbox is not a place worth remembering`() =
        runTest {
            val untouched = FakeRecentProjects(listOf(2L))
            val presenter = presenter(recent = untouched)
            watch(presenter)

            presenter.open()
            presenter.setTitles("Something raw")
            presenter.submit()
            runCurrent()

            assertEquals(TaskDestination.Inbox, actions.calls.single().destination)
            assertEquals(listOf(2L), untouched.remembered)
        }

    /**
     * A date is a plan, and the inbox holds what has not been planned. The
     * control is hidden there, and the value is dropped as well — so a date left
     * behind by switching back to the inbox cannot be sent by accident.
     */
    @Test
    fun `a due date set for a project is dropped if the task goes to the inbox instead`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.chooseProject(1)
            presenter.setTitles("Fix the shelf")
            presenter.setDueDate(LocalDate.of(2026, 3, 4))
            presenter.chooseProject(null)
            presenter.submit()
            runCurrent()

            assertNull(actions.calls.single().task.dueAt)
        }

    @Test
    fun `a due date is sent as the start of that day where the user is`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.chooseProject(1)
            presenter.setTitles("Fix the shelf")
            presenter.setDueDate(LocalDate.of(2026, 3, 4))
            presenter.submit()
            runCurrent()

            val expected = LocalDate.of(2026, 3, 4).atStartOfDay(zone).toInstant().toEpochMilli()
            assertEquals(expected, actions.calls.single().task.dueAt)
            assertFalse(actions.calls.single().task.dueHasTime)
        }

    @Test
    fun `tapping the day already set clears it`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.chooseProject(1)
            presenter.setDueDate(LocalDate.of(2026, 3, 4))
            presenter.setDueDate(LocalDate.of(2026, 3, 4))
            runCurrent()

            assertNull(presenter.state.value.draft.dueDate)
        }

    // --- the part of the day, guessed ---

    @Test
    fun `a task for a project today takes the part of the day it is written in`() =
        runTest {
            val presenter = presenter(now = at(TODAY, 19))
            watch(presenter)

            presenter.open()
            presenter.chooseProject(1)
            presenter.setDueDate(TODAY)
            runCurrent()

            assertEquals(DayPart.EVENING, presenter.state.value.draft.dayPart)
        }

    @Test
    fun `before the day starts the guess is the morning`() =
        runTest {
            val presenter = presenter(now = at(TODAY, 6))
            watch(presenter)

            presenter.open()
            presenter.setDueDate(TODAY)
            presenter.chooseProject(1)
            runCurrent()

            assertEquals(DayPart.MORNING, presenter.state.value.draft.dayPart)
        }

    @Test
    fun `no guess for the inbox or another day, and a guess is taken back when it stops fitting`() =
        runTest {
            val presenter = presenter(now = at(TODAY, 14))
            watch(presenter)

            presenter.open()
            presenter.setDueDate(TODAY)
            runCurrent()
            assertEquals(DayPart.NONE, presenter.state.value.draft.dayPart)

            presenter.chooseProject(1)
            runCurrent()
            assertEquals(DayPart.AFTERNOON, presenter.state.value.draft.dayPart)

            presenter.setDueDate(TODAY.plusDays(1))
            runCurrent()
            assertEquals(DayPart.NONE, presenter.state.value.draft.dayPart)
        }

    @Test
    fun `a part of the day picked by hand is never replaced`() =
        runTest {
            val presenter = presenter(now = at(TODAY, 19))
            watch(presenter)

            presenter.open()
            presenter.setDayPart(DayPart.MORNING)
            presenter.chooseProject(1)
            presenter.setDueDate(TODAY)
            presenter.setDueDate(TODAY.plusDays(1))
            runCurrent()

            assertEquals(DayPart.MORNING, presenter.state.value.draft.dayPart)
        }

    // --- the two title rules ---

    @Test
    fun `the labels a title earns are shown before it is saved`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings = AppSettings(autoLabels = listOf(AutoLabelRule("buy", listOf(1), ignoreCase = true))),
                )
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Buy milk")
            runCurrent()

            assertEquals(listOf("errand"), presenter.state.value.autoLabels)
        }

    @Test
    fun `a label taken off travels with the write so the server does not put it back`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings = AppSettings(autoLabels = listOf(AutoLabelRule("buy", listOf(1), ignoreCase = true))),
                )
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Buy milk")
            presenter.rejectAutoLabel("errand")
            runCurrent()

            assertEquals(emptyList(), presenter.state.value.autoLabels)
            presenter.submit()
            runCurrent()
            assertEquals(listOf("errand"), actions.calls.single().task.removedAutoLabels)
        }

    /**
     * The difference the whole surface turns on: a suggested project is offered
     * and nothing else. Typing a title that matches a rule must not move the task.
     */
    @Test
    fun `a suggested project is offered and never applied`() =
        runTest {
            workspace.value =
                workspace.value.copy(
                    appSettings =
                        AppSettings(
                            projectSuggestions = listOf(ProjectSuggestionRule("garden", listOf(3), ignoreCase = true)),
                        ),
                )
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("garden shed")
            runCurrent()

            assertEquals(listOf("Garden"), presenter.state.value.suggestedProjects.map { it.title })
            presenter.submit()
            runCurrent()
            assertEquals(TaskDestination.Inbox, actions.calls.single().destination)
        }

    // --- the picker ---

    @Test
    fun `the picker leads with the places this device files things, listed once`() =
        runTest {
            val presenter = presenter(recent = FakeRecentProjects(listOf(4L, 2L)))
            watch(presenter)

            presenter.open()
            presenter.setPickingProject(true)
            runCurrent()

            val state = presenter.state.value
            assertEquals(listOf("Music", "Work"), state.recentProjects.map { it.title })
            assertEquals(listOf("Home", "Garden", "Books", "Motor"), state.otherProjects.map { it.title })
        }

    @Test
    fun `projects worked on today are offered only when the sheet was opened from today`() =
        runTest {
            workspace.value = workspace.value.copy(todayProjectLocalIds = listOf(3L, 99L, 2L))
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            runCurrent()
            assertEquals(emptyList<String>(), presenter.state.value.todayProjects.map { it.title })

            presenter.open(QuickAddRequest(todayProjects = true))
            runCurrent()
            assertEquals(listOf("Garden", "Work"), presenter.state.value.todayProjects.map { it.title })

            presenter.chooseProject(3L)
            runCurrent()
            assertEquals(listOf("Garden", "Work"), presenter.state.value.todayProjects.map { it.title })
            assertEquals(3L, presenter.state.value.draft.projectLocalId)
        }

    @Test
    fun `the project filed into last time is offered next to the destination`() =
        runTest {
            val presenter = presenter(recent = FakeRecentProjects(listOf(4L, 2L)))
            watch(presenter)

            presenter.open()
            runCurrent()

            assertEquals("Music", presenter.state.value.lastProject?.title)
        }

    @Test
    fun `the last project is not offered while it is already the destination`() =
        runTest {
            val presenter = presenter(recent = FakeRecentProjects(listOf(4L, 2L)))
            watch(presenter)

            presenter.open(QuickAddRequest(projectLocalId = 4L))
            runCurrent()
            assertNull(presenter.state.value.lastProject)

            presenter.chooseProject(null)
            runCurrent()
            assertEquals("Music", presenter.state.value.lastProject?.title)
        }

    @Test
    fun `nothing is offered when no project was filed into before`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            runCurrent()

            assertNull(presenter.state.value.lastProject)
        }

    @Test
    fun `searching narrows both the recent row and the list behind it`() =
        runTest {
            val presenter = presenter(recent = FakeRecentProjects(listOf(4L, 2L)))
            watch(presenter)

            presenter.open()
            presenter.setPickingProject(true)
            presenter.setProjectQuery("o")
            runCurrent()

            // "Music" holds no `o`, so the narrowing drops it from the row too.
            val state = presenter.state.value
            assertEquals(listOf("Work"), state.recentProjects.map { it.title })
            assertEquals(listOf("Home", "Books", "Motor"), state.otherProjects.map { it.title })
        }

    @Test
    fun `picking a place closes the picker and clears what was typed into it`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setPickingProject(true)
            presenter.setProjectQuery("wor")
            presenter.chooseProject(2)
            runCurrent()

            val state = presenter.state.value
            assertFalse(state.pickingProject)
            assertEquals("", state.projectQuery)
            assertEquals("Work", state.projectTitle)
        }

    // --- when the write path says no ---

    @Test
    fun `a save that fails halfway keeps the sheet open holding only what is left`() =
        runTest {
            actions.failFrom = 3
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("First\nSecond\nThird\nFourth")
            presenter.submit()
            runCurrent()

            assertEquals(listOf("First", "Second"), actions.calls.map { it.task.title })
            val state = presenter.state.value
            assertTrue(state.visible)
            assertEquals("Third\nFourth", state.draft.titles)
            assertFalse(state.submitting)
        }

    @Test
    fun `re-saving after a failure does not write the lines that already landed`() =
        runTest {
            actions.failFrom = 2
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("First\nSecond")
            presenter.submit()
            runCurrent()

            actions.failFrom = Int.MAX_VALUE
            presenter.submit()
            runCurrent()

            assertEquals(listOf("First", "Second"), actions.calls.map { it.task.title })
        }

    @Test
    fun `a share opens the sheet on what was shared rather than on what was left behind`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Half-written")
            presenter.dismiss()
            presenter.open(QuickAddRequest(title = "Shared page", description = "https://example.test"))
            runCurrent()

            val draft = presenter.state.value.draft
            assertEquals("Shared page", draft.titles)
            assertEquals("https://example.test", draft.description)
        }

    @Test
    fun `a title is saved without the dots and spaces at its end`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Buy milk. \nCall v2.5 support...\nSee example.com")
            presenter.submit()
            runCurrent()

            assertEquals(
                listOf("Buy milk", "Call v2.5 support", "See example.com"),
                actions.calls.map { it.task.title },
            )
        }

    @Test
    fun `a title of nothing but dots cannot be saved`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles(" . .. ")
            runCurrent()

            assertFalse(presenter.state.value.draft.canSubmit)
        }

    @Test
    fun `an untouched sheet closes without asking`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open(QuickAddRequest(projectLocalId = 1))
            runCurrent()
            assertFalse(presenter.state.value.holdsInput)

            presenter.requestDismiss()
            runCurrent()

            assertFalse(presenter.state.value.visible)
        }

    @Test
    fun `a sheet holding anything asks before it closes`() =
        runTest {
            val fills =
                listOf<(QuickAddPresenter) -> Unit>(
                    { it.setTitles("Buy milk") },
                    { it.setDescription("Oat") },
                    { it.setPriority(Priority.HIGH) },
                    { it.setDayPart(DayPart.EVENING) },
                    { it.setLabels(listOf("errand")) },
                    { it.chooseProject(2) },
                )
            for (fill in fills) {
                val presenter = presenter()
                watch(presenter)
                presenter.open()
                fill(presenter)
                presenter.requestDismiss()
                runCurrent()

                assertTrue(presenter.state.value.visible)
                assertTrue(presenter.state.value.confirmingDiscard)
            }
        }

    @Test
    fun `text that arrived in a share counts as something to lose`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open(QuickAddRequest(title = "Shared page"))
            presenter.requestDismiss()
            runCurrent()

            assertTrue(presenter.state.value.confirmingDiscard)
        }

    @Test
    fun `keeping on editing leaves the draft as it was, and discarding forgets it`() =
        runTest {
            val presenter = presenter()
            watch(presenter)

            presenter.open()
            presenter.setTitles("Buy milk")
            presenter.requestDismiss()
            presenter.keepEditing()
            runCurrent()

            assertFalse(presenter.state.value.confirmingDiscard)
            assertTrue(presenter.state.value.visible)
            assertEquals("Buy milk", presenter.state.value.draft.titles)

            presenter.requestDismiss()
            presenter.dismiss()
            runCurrent()

            assertFalse(presenter.state.value.visible)
            assertEquals("", presenter.state.value.draft.titles)
        }

    private fun at(
        day: LocalDate,
        hour: Int,
    ): Instant = LocalDateTime.of(day, LocalTime.of(hour, 0)).atZone(zone).toInstant()

    private companion object {
        /** A day none of the other checks set a due date on, so no guess reaches them. */
        val TODAY: LocalDate = LocalDate.of(2026, 5, 20)
        val NOW: Instant = LocalDateTime.of(2026, 5, 20, 12, 0).atZone(ZoneId.of("Europe/Berlin")).toInstant()
    }
}
