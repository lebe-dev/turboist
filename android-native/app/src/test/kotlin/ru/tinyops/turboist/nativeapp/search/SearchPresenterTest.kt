package ru.tinyops.turboist.nativeapp.search

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.test.StandardTestDispatcher
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Test
import ru.tinyops.turboist.nativeapp.tasks.task
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * How the search screen behaves, with nothing behind it.
 *
 * Every case here is about a decision rather than about a query: when a search is
 * worth running, when it is worth waiting for the next keystroke, and what counts
 * as a search worth remembering. The index has its own tests; this one would pass
 * just as well if the index answered nonsense, which is exactly the separation
 * that makes both readable.
 */
@OptIn(ExperimentalCoroutinesApi::class)
class SearchPresenterTest {
    private val repository = RecordingSearchRepository()
    private val recent = FakeRecentSearches()

    /**
     * A presenter on a scope of the test's own, sharing the test's clock.
     *
     * Not the test scope itself: the presenter watches the recent-search list for
     * as long as it lives, and a watcher that never finishes would keep a test
     * that owns it from ever finishing either.
     */
    private fun TestScope.presenterWith(
        repository: SearchRepository = this@SearchPresenterTest.repository,
        recent: RecentSearches = this@SearchPresenterTest.recent,
    ): SearchPresenter = SearchPresenter(CoroutineScope(StandardTestDispatcher(testScheduler)), repository, recent)

    @Test
    fun `a query too short to be worth running never reaches the replica`() =
        runTest {
            val presenter = presenterWith()

            presenter.type("a")
            advanceUntilIdle()

            assertEquals(emptyList(), repository.calls)
            assertTrue(presenter.state.value.resting)
            assertFalse(presenter.state.value.answered)
        }

    @Test
    fun `a word typed at speed is one search rather than one per letter`() =
        runTest {
            val presenter = presenterWith()

            presenter.type("pa")
            advanceTimeBy(100)
            presenter.type("pas")
            advanceTimeBy(100)
            presenter.type("pass")
            advanceUntilIdle()

            assertEquals(listOf(SearchCall("pass", SearchFilters())), repository.calls)
        }

    /**
     * Each change is answered on its own here. Two in the same instant would be
     * one search, not two — a run in flight is abandoned the moment the question
     * changes — and that is checked by the case above, in the form a person
     * actually produces it: keystrokes.
     */
    @Test
    fun `changing a filter searches at once, with the filter applied`() =
        runTest {
            val presenter = presenterWith()
            presenter.type("passport")
            advanceUntilIdle()
            repository.calls.clear()

            presenter.narrowTo(SearchKind.PROJECTS)
            advanceUntilIdle()
            presenter.toggleOpenTasksOnly()
            advanceUntilIdle()

            assertEquals(
                listOf(
                    SearchCall("passport", SearchFilters(kind = SearchKind.PROJECTS)),
                    SearchCall("passport", SearchFilters(kind = SearchKind.PROJECTS, openTasksOnly = true)),
                ),
                repository.calls,
            )
        }

    @Test
    fun `what was found reaches the screen, and is marked as an answer`() =
        runTest {
            repository.answer = SearchResults(tasks = listOf(TaskHit(task(1, title = "Renew passport"), null)))
            val presenter = presenterWith()

            presenter.type("passport")
            advanceUntilIdle()

            val state = presenter.state.value
            assertEquals(1, state.results.total)
            assertTrue(state.answered)
            assertFalse(state.searching)
        }

    @Test
    fun `only a deliberate search is remembered`() =
        runTest {
            val presenter = presenterWith()

            presenter.type("passport")
            advanceUntilIdle()
            assertEquals(emptyList(), presenter.state.value.recent)

            presenter.submit()
            advanceUntilIdle()

            assertEquals(listOf("passport"), presenter.state.value.recent)
        }

    @Test
    fun `tapping a past search fills the field and runs it`() =
        runTest {
            val presenter = presenterWith(recent = FakeRecentSearches(listOf("passport")))
            advanceUntilIdle()
            assertEquals(listOf("passport"), presenter.state.value.recent)

            presenter.rerun("passport")
            advanceUntilIdle()

            assertEquals("passport", presenter.state.value.typed)
            assertEquals(listOf(SearchCall("passport", SearchFilters())), repository.calls)
        }

    @Test
    fun `emptying the field puts the screen back where it started`() =
        runTest {
            repository.answer = SearchResults(tasks = listOf(TaskHit(task(1, title = "Renew passport"), null)))
            val presenter = presenterWith()
            presenter.type("passport")
            advanceUntilIdle()

            presenter.clearQuery()
            advanceUntilIdle()

            val state = presenter.state.value
            assertTrue(state.resting)
            assertFalse(state.answered)
            assertTrue(state.results.isEmpty)
        }

    @Test
    fun `forgetting the past searches empties the list`() =
        runTest {
            val presenter = presenterWith(recent = FakeRecentSearches(listOf("passport")))
            advanceUntilIdle()

            presenter.forgetRecent()
            advanceUntilIdle()

            assertEquals(emptyList(), presenter.state.value.recent)
        }
}
