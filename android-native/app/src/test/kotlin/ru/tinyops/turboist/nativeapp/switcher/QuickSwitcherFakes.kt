package ru.tinyops.turboist.nativeapp.switcher

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow

/** Remembered recents, in memory. */
class FakeRecentItems(
    initial: List<RecentRef> = emptyList(),
) : RecentItems {
    private val order = MutableStateFlow(initial)

    val remembered: List<RecentRef> get() = order.value

    override fun observe(): Flow<List<RecentRef>> = order

    override suspend fun visit(ref: RecentRef) {
        order.value = withRecentItem(order.value, ref)
    }

    override suspend fun clear() {
        order.value = emptyList()
    }
}

/** A workspace of fixed rows: [search] answers from [matches], [resolve] from [rows]. */
class FakeQuickSwitcherRepository(
    private val rows: List<SwitcherRow> = emptyList(),
    private val matches: Map<String, SwitcherMatches> = emptyMap(),
) : QuickSwitcherRepository {
    val searches = mutableListOf<String>()

    override suspend fun search(typed: String): SwitcherMatches {
        searches += typed
        return matches[typed] ?: SwitcherMatches()
    }

    override suspend fun resolve(refs: List<RecentRef>): List<SwitcherRow> {
        val byRef = rows.associateBy { it.ref }
        return refs.mapNotNull(byRef::get)
    }
}
