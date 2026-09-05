package ru.tinyops.turboist.nativeapp.search

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow

/** One question the screen asked, recorded so a test can say what was asked and how often. */
data class SearchCall(
    val typed: String,
    val filters: SearchFilters,
)

/**
 * The replica as far as the search screen can tell.
 *
 * It records what it was asked and answers with whatever the test set, which is
 * everything a test of the screen's behaviour needs: what the index would return
 * is settled where the index lives, not here.
 */
class RecordingSearchRepository(
    var answer: SearchResults = SearchResults(),
) : SearchRepository {
    val calls = mutableListOf<SearchCall>()

    override suspend fun search(
        typed: String,
        filters: SearchFilters,
    ): SearchResults {
        calls += SearchCall(typed, filters)
        return answer
    }
}

/** The recent-search list, in memory. */
class FakeRecentSearches(
    initial: List<String> = emptyList(),
) : RecentSearches {
    private val queries = MutableStateFlow(initial)

    override fun observe(): Flow<List<String>> = queries

    override suspend fun remember(query: String) {
        queries.value = withRecentSearch(queries.value, query)
    }

    override suspend fun clear() {
        queries.value = emptyList()
    }
}
