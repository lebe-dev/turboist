package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.collectLatest
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.dao.LabelSuggestionDao
import ru.tinyops.turboist.core.database.dao.TextEmbeddingDao
import ru.tinyops.turboist.core.database.entity.TextEmbeddingRow
import ru.tinyops.turboist.core.model.labelsuggest.LabelScorer
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionIndex
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe
import ru.tinyops.turboist.core.model.labelsuggest.ProjectScorer
import ru.tinyops.turboist.core.model.labelsuggest.SuggestionTexts
import ru.tinyops.turboist.core.model.labelsuggest.Vectors
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Label and project suggestions on this phone: keeps an index of the workspace
 * embedded, and answers both kinds of question from it.
 *
 * The index follows the replica. Whenever a task, label or project changes —
 * a sync, an edit, a deletion — it is rebuilt a moment later from the replica
 * and the embedding cache, so only what is new goes through the model: after
 * the first pass that is a handful of titles at most. The first pass embeds
 * every task on the device, newest first, and the index is published after
 * each slice so suggestions start working long before it finishes.
 *
 * One index and one engine serve both kinds, since both read the same titles
 * through the same model. It runs while either kind is on and the model is
 * here; turning both off drops the index and closes the engine. Each kind
 * answers only while its own switch is on.
 */
@Singleton
class OnDeviceSuggestions internal constructor(
    private val database: TurboistDatabase,
    switches: Flow<SuggestionSwitches>,
    private val model: ModelStore,
    private val embedder: TextEmbedder,
    private val scope: CoroutineScope,
    private val rebuildDelayMillis: Long = REBUILD_DELAY_MILLIS,
) {
    @Inject
    constructor(
        database: TurboistDatabase,
        preference: SuggestionPreference,
        model: ModelStore,
        embedder: LiteRtEmbedder,
        @ApplicationScope scope: CoroutineScope,
    ) : this(
        database = database,
        // A phone the engine cannot run on is one where suggestions are simply off.
        switches = preference.switches.map { if (LabelSuggestionSupport.supported) it else SuggestionSwitches() },
        model = model,
        embedder = embedder,
        scope = scope,
    )

    private val dao: LabelSuggestionDao get() = database.labelSuggestions()
    private val cache: TextEmbeddingDao get() = database.textEmbeddings()
    private val index = MutableStateFlow<LabelSuggestionIndex?>(null)
    private val learned = MutableStateFlow(IndexProgress())

    private val switched: StateFlow<SuggestionSwitches> =
        switches.stateIn(scope, SharingStarted.Eagerly, SuggestionSwitches())
    private val switchedOn: Flow<Boolean> = switches.map { it.any }

    /**
     * Every vector this process has read or computed, by text, so a rebuild
     * after a single edit reads nothing from the cache table. Only touched
     * from [rebuild], which never runs twice at once.
     */
    private val vectors = HashMap<String, FloatArray>()

    /**
     * The last title put to the model and its vector. The capture sheet asks
     * about labels and projects for the same title at the same moment; the
     * second question reuses the first one's answer instead of waiting for the
     * model again.
     */
    @Volatile private var lastQuery: Pair<String, FloatArray>? = null
    private val queryLock = Mutex()

    /** How much of the device's history the index has taken in, for the settings screen to show. */
    val progress: StateFlow<IndexProgress> = learned

    private val enabled: Flow<Boolean> =
        combine(switchedOn, model.state) { on, state ->
            on && state == ModelState.Ready
        }
            .distinctUntilChanged()

    private val live: Flow<LabelSuggestionIndex?> =
        combine(enabled, index) { on, current -> current?.takeIf { on } }

    /** Labels suggested for a title. */
    val labels: LabelSuggestions =
        object : LabelSuggestions {
            override val available: StateFlow<Boolean> =
                combine(switched, live) { on, current -> on.labels && current != null && !current.isEmpty }
                    .stateIn(scope, SharingStarted.Eagerly, false)

            override suspend fun suggest(request: SuggestionRequest): List<Long> = suggestLabels(request)
        }

    /** Projects suggested as the place for a title. */
    val projects: ProjectSuggestions =
        object : ProjectSuggestions {
            override val available: StateFlow<Boolean> =
                combine(switched, live) { on, current -> on.projects && current?.projects?.any { it.open } == true }
                    .stateIn(scope, SharingStarted.Eagerly, false)

            override suspend fun suggest(request: ProjectSuggestionRequest): List<Long> = suggestProjects(request)
        }

    /** Starts following the replica. Called once, when the process starts. */
    @OptIn(FlowPreview::class)
    fun start() {
        scope.launch {
            // A download the process was killed in the middle of carries on
            // once the app is opened again, from where it stopped.
            if (switchedOn.first() && model.state.value !is ModelState.Ready) model.download()
        }
        scope.launch {
            enabled.collectLatest { on ->
                if (!on) {
                    index.value = null
                    vectors.clear()
                    lastQuery = null
                    learned.value = IndexProgress()
                    embedder.release()
                    return@collectLatest
                }
                database.invalidationTracker
                    .createFlow("tasks", "task_labels", "labels", "projects", "project_labels")
                    .debounce(rebuildDelayMillis)
                    .collectLatest { rebuild() }
            }
        }
    }

    private suspend fun suggestLabels(request: SuggestionRequest): List<Long> {
        if (!switched.value.labels || request.title.isBlank()) return emptyList()
        val current = index.value?.takeUnless { it.isEmpty } ?: return emptyList()
        val query = embedQuery(request.title) ?: return emptyList()
        val scores =
            LabelScorer.score(
                query = query,
                index = current,
                projectId = request.projectLocalId,
                now = System.currentTimeMillis(),
                excludeTaskId = request.taskLocalId,
            )
        val minScore =
            when (request.surface) {
                SuggestionSurface.CAPTURE -> LabelSuggestionRecipe.CAPTURE_THRESHOLD
                SuggestionSurface.PICKER -> LabelSuggestionRecipe.PICKER_FLOOR
            }
        return LabelScorer.pick(scores, request.exclude, minScore, request.limit)
    }

    private suspend fun suggestProjects(request: ProjectSuggestionRequest): List<Long> {
        if (!switched.value.projects || request.title.isBlank()) return emptyList()
        val current = index.value ?: return emptyList()
        if (current.projects.none { it.open }) return emptyList()
        val query = embedQuery(request.title) ?: return emptyList()
        val scores = ProjectScorer.score(query = query, index = current, now = System.currentTimeMillis())
        return ProjectScorer.pick(scores, request.exclude, max = request.limit)
    }

    private suspend fun embedQuery(title: String): FloatArray? =
        queryLock.withLock {
            val text = SuggestionTexts.query(title)
            lastQuery?.takeIf { it.first == text }?.let { return@withLock it.second }
            val vector = embedder.embed(listOf(text)).single() ?: return@withLock null
            lastQuery = text to vector
            vector
        }

    /**
     * Brings the index up to date with the replica, embedding only texts the
     * cache does not hold yet, and drops cached vectors nothing uses any more.
     */
    private suspend fun rebuild() {
        val plan = SuggestionIndexPlan(withContext(Dispatchers.IO) { dao.workspace() })
        if (plan.allTexts.any { it !in vectors }) {
            val cached = withContext(Dispatchers.IO) { cache.embeddings() }.associateBy { it.key }
            for (text in plan.allTexts) {
                if (text in vectors) continue
                cached[SuggestionIndexPlan.key(text)]?.let { vectors[text] = Vectors.fromBytes(it.vector) }
            }
        }
        // Labels and projects lead the list, so the first slice makes the index usable.
        for (slice in plan.allTexts.filter { it !in vectors }.chunked(SLICE)) {
            embedInto(vectors, slice)
            publish(plan, vectors)
        }
        publish(plan, vectors)
        sweep(plan)
    }

    /**
     * Embeds [slice] into [vectors] and the cache, a few texts at a time, so
     * a title typed meanwhile is never queued behind a whole slice.
     *
     * Each batch is stored before it is remembered: a rebuild cancelled by the
     * next change carries on from [vectors], so a vector held only in memory
     * would never reach the cache and would be worked out again on every start.
     */
    private suspend fun embedInto(
        vectors: MutableMap<String, FloatArray>,
        slice: List<String>,
    ) {
        for (batch in slice.chunked(BATCH)) {
            val embedded =
                batch.zip(embedder.embed(batch)).mapNotNull { (text, vector) -> vector?.let { text to it } }
            if (embedded.isEmpty()) continue
            val rows =
                embedded.map {
                        (text, vector) ->
                    TextEmbeddingRow(SuggestionIndexPlan.key(text), Vectors.toBytes(vector))
                }
            withContext(Dispatchers.IO) { cache.putEmbeddings(rows) }
            vectors.putAll(embedded)
        }
    }

    private fun publish(
        plan: SuggestionIndexPlan,
        vectors: Map<String, FloatArray>,
    ) {
        val built = plan.build(vectors::get)
        index.value = built
        learned.value = IndexProgress(learned = built.history.size, total = plan.taskTexts.size)
    }

    private suspend fun sweep(plan: SuggestionIndexPlan) {
        val texts = plan.allTexts.toHashSet()
        vectors.keys.retainAll(texts)
        val wanted = texts.mapTo(HashSet()) { SuggestionIndexPlan.key(it) }
        withContext(Dispatchers.IO) {
            val stale = cache.embeddingKeys().filter { it !in wanted }
            stale.chunked(SWEEP_CHUNK).forEach { cache.deleteEmbeddings(it) }
        }
    }

    private companion object {
        /** A sync lands as a burst of writes; one rebuild after it settles is enough. */
        const val REBUILD_DELAY_MILLIS = 2_000L

        /** How many texts are embedded between two publications of the index. */
        const val SLICE = 128

        /** How many texts go to the engine in one call. */
        const val BATCH = 16

        /** Under SQLite's limit on the number of values in one statement. */
        const val SWEEP_CHUNK = 500
    }
}

/** How many of the device's tasks the index has learned from, out of how many there are. */
data class IndexProgress(
    val learned: Int = 0,
    val total: Int = 0,
)
