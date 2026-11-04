package ru.tinyops.turboist.nativeapp.switcher

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.sync.write.TaskDestination
import ru.tinyops.turboist.nativeapp.quickadd.RecordingQuickAddActions
import ru.tinyops.turboist.nativeapp.tasks.task
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
    ) = QuickSwitcherPresenter(
        scope = CoroutineScope(StandardTestDispatcher(testScheduler)),
        repository = repository,
        recentItems = recent,
        actions = actions,
        zone = zone,
        today = { today },
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
    fun `for today files into the most recently opened open project, due at the start of today`() =
        runTest {
            val recent = FakeRecentItems(listOf(archived.ref, passport.ref, dacha.ref))
            val presenter = presenterWith(FakeQuickSwitcherRepository(rows = listOf(archived, passport, dacha)), recent)

            presenter.open()
            presenter.type("  Buy   seeds ")
            advanceUntilIdle()
            assertEquals("Dacha", presenter.state.value.todayProject?.title)

            presenter.create(SwitcherTarget.TODAY)
            advanceUntilIdle()

            val call = actions.calls.single()
            assertEquals(TaskDestination.InProject(5), call.destination)
            assertEquals("Buy seeds", call.task.title)
            assertEquals(today.atStartOfDay(zone).toInstant().toEpochMilli(), call.task.dueAt)
            assertEquals(QuickSwitcherMessage.AddedForToday("Dacha"), presenter.messages.first())
            assertFalse(presenter.state.value.visible)
        }

    @Test
    fun `for today with no recent project goes to the inbox, still dated`() =
        runTest {
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Buy seeds")
            advanceUntilIdle()
            assertNull(presenter.state.value.todayProject)
            presenter.create(SwitcherTarget.TODAY)
            advanceUntilIdle()

            val call = actions.calls.single()
            assertEquals(TaskDestination.Inbox, call.destination)
            assertEquals(today.atStartOfDay(zone).toInstant().toEpochMilli(), call.task.dueAt)
        }

    @Test
    fun `to the inbox files an undated task`() =
        runTest {
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Call mom")
            presenter.create(SwitcherTarget.INBOX)
            advanceUntilIdle()

            val call = actions.calls.single()
            assertEquals(TaskDestination.Inbox, call.destination)
            assertNull(call.task.dueAt)
            assertEquals(QuickSwitcherMessage.AddedToInbox, presenter.messages.first())
        }

    @Test
    fun `a failed write keeps the panel open with what was typed`() =
        runTest {
            actions.failFrom = 1
            val presenter = presenterWith(FakeQuickSwitcherRepository(), FakeRecentItems())

            presenter.open()
            presenter.type("Call mom")
            presenter.create(SwitcherTarget.INBOX)
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
}
