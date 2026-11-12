package ru.tinyops.turboist.nativeapp.projects

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.flatMapLatest
import kotlinx.coroutines.flow.flowOf
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.Context
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.model.view.LabelUsagePeriod
import ru.tinyops.turboist.core.model.view.ProjectActivity
import ru.tinyops.turboist.core.model.view.ProjectRadar
import ru.tinyops.turboist.core.model.view.projectRadar
import ru.tinyops.turboist.core.sync.write.NewProject
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler

/** Everything the projects screen renders. */
data class ProjectsUiState(
    val loading: Boolean = true,
    val filter: ProjectFilter = ProjectFilter.ALL,
    val query: String = "",
    /** The workspace as the screen draws it: a heading per context, its projects under it. */
    val groups: List<ProjectGroup> = emptyList(),
    /** The open projects the user keeps to hand, whichever context they live in. */
    val pinned: List<Project> = emptyList(),
    /** How many projects each narrowing would show, so the chips can carry a number. */
    val counts: Map<ProjectFilter, Int> = emptyMap(),
    /** The contexts a new project can be filed under. */
    val contexts: List<Context> = emptyList(),
    /** The labels a new project can be given. */
    val labels: List<Label> = emptyList(),
    /**
     * Whether the daily plan is part of this installation's product. With it off
     * the plan does not exist for this user, so nothing here says a project
     * stands in one.
     */
    val dailyPlanEnabled: Boolean = false,
    /** Whether the activity radar is open. It starts closed: the list is what the screen is for. */
    val radarExpanded: Boolean = false,
    val radarPeriod: LabelUsagePeriod = LabelUsagePeriod.WEEK,
    /**
     * Where the finished work went, project by project; null while the radar is
     * closed, and when too few projects finished anything to make a shape.
     */
    val radar: ProjectRadar? = null,
    val refreshing: Boolean = false,
) {
    /** True once the workspace is known to hold nothing matching, as opposed to not being known yet. */
    val isEmpty: Boolean get() = !loading && groups.all { it.projects.isEmpty() }
}

/**
 * The standing reads the projects screen is drawn from: the workspace, the one
 * preference that changes how it is drawn, and the finished work the activity
 * radar counts (only collected while the radar is open).
 */
class ProjectsSources(
    val groups: Flow<List<ProjectGroup>>,
    val contexts: Flow<List<Context>>,
    val dailyPlanEnabled: Flow<Boolean>,
    val labels: Flow<List<Label>> = flowOf(emptyList()),
    val activity: Flow<List<ProjectActivity>> = flowOf(emptyList()),
)

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
@OptIn(ExperimentalCoroutinesApi::class)
class ProjectsPresenter(
    private val scope: CoroutineScope,
    sources: ProjectsSources,
    private val actions: ProjectActions,
    private val sync: SyncScheduler,
) {
    private val filter = MutableStateFlow(ProjectFilter.ALL)
    private val query = MutableStateFlow("")
    private val refreshing = MutableStateFlow(false)
    private val radarExpanded = MutableStateFlow(false)
    private val radarPeriod = MutableStateFlow(LabelUsagePeriod.WEEK)
    private val outgoing = MutableSharedFlow<ProjectMessage>(extraBufferCapacity = 1)

    /** The workspace and the one preference that changes how it is drawn, read together. */
    private val workspace =
        combine(
            sources.groups,
            sources.contexts,
            sources.dailyPlanEnabled,
            sources.labels,
        ) { groups, contexts, plan, labels ->
            Workspace(groups, contexts, plan, labels)
        }

    /**
     * The radar, once it is open. While it is closed the counting query is not
     * collected at all: it scans every finished task, and nobody is looking.
     */
    private val radar: Flow<ProjectRadar?> =
        combine(radarExpanded, radarPeriod, ::Pair).flatMapLatest { (open, period) ->
            if (open) {
                sources.activity.map { rows -> projectRadar(rows, period) }
            } else {
                flowOf(null)
            }
        }

    /** What the screen renders. */
    val state: StateFlow<ProjectsUiState> =
        combine(
            workspace,
            filter,
            query,
            combine(refreshing, radarExpanded, radarPeriod, radar, ::RadarState),
        ) { known, chosen, typed, extras ->
            val all = known.groups
            ProjectsUiState(
                loading = false,
                filter = chosen,
                query = typed,
                groups = narrow(all, chosen, typed),
                pinned = if (typed.isBlank()) pinned(all, chosen) else emptyList(),
                counts = ProjectFilter.entries.associateWith { candidate -> count(all, candidate) },
                contexts = known.contexts,
                labels = known.labels,
                dailyPlanEnabled = known.dailyPlanEnabled,
                radarExpanded = extras.expanded,
                radarPeriod = extras.period,
                radar = extras.radar,
                refreshing = extras.refreshing,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), ProjectsUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<ProjectMessage> = outgoing.asSharedFlow()

    /** The user chose which slice of the workspace to look at. */
    fun narrowTo(chosen: ProjectFilter) {
        filter.value = chosen
    }

    /** The user opened or closed the activity radar. */
    fun toggleRadar() {
        radarExpanded.value = !radarExpanded.value
    }

    /** The user chose how far back the radar looks. */
    fun chooseRadarPeriod(chosen: LabelUsagePeriod) {
        radarPeriod.value = chosen
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
        description: String = "",
        labels: List<String> = emptyList(),
    ) {
        val named = title.trim()
        if (named.isEmpty()) return
        val project = NewProject(title = named, description = description.trim(), labels = labels)
        scope.launch { outgoing.reporting { actions.createProject(contextLocalId, project) } }
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

    /**
     * The pinned row. While the narrowing spans every status only open projects
     * are on it — a finished project has nothing left to keep to hand — and a
     * narrowing to one status shows the pinned projects standing there instead.
     * A search empties it: the matches are listed in their contexts, and a pinned
     * row above them would show the same project twice or one that does not match.
     */
    private fun pinned(
        all: List<ProjectGroup>,
        chosen: ProjectFilter,
    ): List<Project> =
        all
            .flatMap { it.projects }
            .filter { it.isPinned && if (chosen.spansStatuses) it.status == ProjectStatus.OPEN else chosen.matches(it) }
            .sortedBy { it.pinnedAt }

    private fun count(
        all: List<ProjectGroup>,
        candidate: ProjectFilter,
    ): Int = all.sumOf { group -> group.projects.count(candidate::matches) }

    /** The pieces of the state that are not about the list, read together. */
    private data class RadarState(
        val refreshing: Boolean,
        val expanded: Boolean,
        val period: LabelUsagePeriod,
        val radar: ProjectRadar?,
    )

    private companion object {
        /**
         * How long the queries behind the screen stay open after it stops being
         * watched. Long enough to survive a rotation, short enough that a screen
         * left behind stops observing the replica.
         */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}

/** The standing reads the screen is drawn from, taken together. */
private data class Workspace(
    val groups: List<ProjectGroup>,
    val contexts: List<Context>,
    val dailyPlanEnabled: Boolean,
    val labels: List<Label>,
)
