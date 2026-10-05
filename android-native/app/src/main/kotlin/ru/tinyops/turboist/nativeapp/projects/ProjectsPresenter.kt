package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.sync.write.NewProject
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler

/** Everything the projects screen renders. */
data class ProjectsUiState(
    val loading: Boolean = true,
    val filter: ProjectFilter = ProjectFilter.ALL,
    val query: String = "",
    /** The workspace as the screen draws it: a heading per context, its projects under it. */
    val groups: List<ProjectGroup> = emptyList(),
    /** The projects the user keeps to hand, whichever context they live in. */
    val pinned: List<Project> = emptyList(),
    /** How many projects each narrowing would show, so the chips can carry a number. */
    val counts: Map<ProjectFilter, Int> = emptyMap(),
    /** The contexts a new project can be filed under. */
    val contexts: List<Context> = emptyList(),
    /**
     * Whether the daily plan is part of this installation's product. With it off
     * the plan does not exist for this user, so nothing here says a project
     * stands in one.
     */
    val dailyPlanEnabled: Boolean = false,
    val refreshing: Boolean = false,
) {
    /** True once the workspace is known to hold nothing matching, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && groups.all { it.projects.isEmpty() }
}

/**
 * The behaviour of the projects screen.
 *
 * A plain object driven by a scope rather than a view model, so all of it can be
 * exercised without Compose and without the platform.
 *
 * The narrowing and the search are applied here rather than in a query, and that
 * is deliberate: a workspace holds tens of projects, the whole set is already in
 * memory as a standing query, and re-running a database query per keystroke
 * would tear that query down and set it up again for an answer it already holds.
 */
class ProjectsPresenter(
    private val scope: CoroutineScope,
    groups: Flow<List<ProjectGroup>>,
    contexts: Flow<List<Context>>,
    dailyPlanEnabled: Flow<Boolean>,
    private val actions: ProjectActions,
    private val sync: SyncScheduler,
) {
    private val filter = MutableStateFlow(ProjectFilter.ALL)
    private val query = MutableStateFlow("")
    private val refreshing = MutableStateFlow(false)
    private val outgoing = MutableSharedFlow<ProjectMessage>(extraBufferCapacity = 1)

    /** The workspace and the one preference that changes how it is drawn, read together. */
    private val workspace = combine(groups, contexts, dailyPlanEnabled, ::Triple)

    /** What the screen renders. */
    val state: StateFlow<ProjectsUiState> =
        combine(
            workspace,
            filter,
            query,
            refreshing,
        ) { (all, knownContexts, planEnabled), chosen, typed, isRefreshing ->
            ProjectsUiState(
                loading = false,
                filter = chosen,
                query = typed,
                groups = narrow(all, chosen, typed),
                pinned = all.flatMap { it.projects }.filter { it.isPinned }.sortedBy { it.pinnedAt },
                counts = ProjectFilter.entries.associateWith { candidate -> count(all, candidate) },
                contexts = knownContexts,
                dailyPlanEnabled = planEnabled,
                refreshing = isRefreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), ProjectsUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<ProjectMessage> = outgoing.asSharedFlow()

    /** The user chose which slice of the workspace to look at. */
    fun narrowTo(chosen: ProjectFilter) {
        filter.value = chosen
    }

    /** The user typed into the search field. */
    fun search(typed: String) {
        query.value = typed
    }

    /**
     * Asks the sync engine to catch up.
     *
     * The list is not refetched — it cannot be, it is a query — so what the
     * gesture asks for is a catch-up, and the rows change because the catch-up
     * wrote to the replica underneath them.
     */
    fun refresh() {
        scope.launch {
            refreshing.value = true
            try {
                sync.requestSyncNow()
            } finally {
                refreshing.value = false
            }
        }
    }

    /** Starts a project inside a context. */
    fun createProject(
        contextLocalId: Long,
        title: String,
    ) {
        val named = title.trim()
        if (named.isEmpty()) return
        scope.launch { outgoing.reporting { actions.createProject(contextLocalId, NewProject(title = named)) } }
    }

    private fun narrow(
        all: List<ProjectGroup>,
        chosen: ProjectFilter,
        typed: String,
    ): List<ProjectGroup> {
        val needle = typed.trim()
        return all.map { group ->
            group.copy(
                projects =
                    projectsInReadingOrder(
                        group.projects.filter { chosen.matches(it) && it.title.contains(needle, ignoreCase = true) },
                        chosen,
                    ),
            )
        }
    }

    private fun count(
        all: List<ProjectGroup>,
        candidate: ProjectFilter,
    ): Int = all.sumOf { group -> group.projects.count(candidate::matches) }

    private companion object {
        /**
         * How long the queries behind the screen stay open after it stops being
         * watched. Long enough to survive a rotation, short enough that a screen
         * left behind stops observing the replica.
         */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
