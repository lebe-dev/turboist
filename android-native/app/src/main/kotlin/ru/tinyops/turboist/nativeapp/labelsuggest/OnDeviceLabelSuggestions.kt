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
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.dao.LabelSuggestionDao
import ru.tinyops.turboist.core.database.dao.TextEmbeddingDao
import ru.tinyops.turboist.core.database.entity.TextEmbeddingRow
import ru.tinyops.turboist.core.model.labelsuggest.LabelScorer
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionIndex
import ru.tinyops.turboist.core.model.labelsuggest.LabelSuggestionRecipe
import ru.tinyops.turboist.core.model.labelsuggest.SuggestionTexts
import ru.tinyops.turboist.core.model.labelsuggest.Vectors
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Label suggestions on this phone: keeps an index of the workspace embedded,
 * and answers questions from it.
 *
 * The index follows the replica. Whenever a task, label or project changes —
 * a sync, an edit, a deletion — it is rebuilt a moment later from the replica
 * and the embedding cache, so only what is new goes through the model: after
 * the first pass that is a handful of titles at most. The first pass embeds
 * every task on the device, newest first, and the index is published after
 * each slice so suggestions start working long before it finishes.
 *
 * Nothing here runs while suggestions are off or the model is missing, and
 * turning them off drops the index and closes the engine.
 */
@Singleton
class OnDeviceLabelSuggestions
    @Inject
    constructor(
        private val database: TurboistDatabase,
        private val preference: LabelSuggestionPreference,
        private val model: ModelStore,
        private val embedder: LiteRtEmbedder,
        @param:ApplicationScope private val scope: CoroutineScope,
    ) : LabelSuggestions {
        private val dao: LabelSuggestionDao get() = database.labelSuggestions()
        private val cache: TextEmbeddingDao get() = database.textEmbeddings()
        private val index = MutableStateFlow<LabelSuggestionIndex?>(null)
        private val learned = MutableStateFlow(IndexProgress())

        /**
         * Every vector this process has read or computed, by text, so a rebuild
         * after a single edit reads nothing from the cache table. Only touched
         * from [rebuild], which never runs twice at once.
         */
        private val vectors = HashMap<String, FloatArray>()

        /** How much of the device's history the index has taken in, for the settings screen to show. */
        val progress: StateFlow<IndexProgress> = learned

        private val enabled: Flow<Boolean> =
            combine(preference.enabled, model.state) { on, state ->
                on && state == ModelState.Ready && LabelSuggestionSupport.supported
            }
                .distinctUntilChanged()

        override val available: StateFlow<Boolean> =
            combine(enabled, index) { on, current -> on && current != null && !current.isEmpty }
                .stateIn(scope, SharingStarted.Eagerly, false)

        /** Starts following the replica. Called once, when the process starts. */
        @OptIn(FlowPreview::class)
        fun start() {
            scope.launch {
                // A download the process was killed in the middle of carries on
                // once the app is opened again, from where it stopped.
                if (!LabelSuggestionSupport.supported) return@launch
                if (preference.enabled.first() && model.state.value !is ModelState.Ready) model.download()
            }
            scope.launch {
                enabled.collectLatest { on ->
                    if (!on) {
                        index.value = null
                        vectors.clear()
                        learned.value = IndexProgress()
                        embedder.release()
                        return@collectLatest
                    }
                    database.invalidationTracker
                        .createFlow("tasks", "task_labels", "labels", "projects", "project_labels")
                        .debounce(REBUILD_DELAY_MILLIS)
                        .collectLatest { rebuild() }
                }
            }
        }

        override suspend fun suggest(request: SuggestionRequest): List<Long> {
            val current = index.value ?: return emptyList()
            if (current.isEmpty || request.title.isBlank()) return emptyList()
            val query = embedder.embed(listOf(SuggestionTexts.query(request.title))).single() ?: return emptyList()
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
         */
        private suspend fun embedInto(
            vectors: MutableMap<String, FloatArray>,
            slice: List<String>,
        ) {
            val rows = ArrayList<TextEmbeddingRow>(slice.size)
            for (batch in slice.chunked(BATCH)) {
                batch.zip(embedder.embed(batch)).forEach { (text, vector) ->
                    if (vector == null) return@forEach
                    vectors[text] = vector
                    rows += TextEmbeddingRow(SuggestionIndexPlan.key(text), Vectors.toBytes(vector))
                }
            }
            withContext(Dispatchers.IO) { cache.putEmbeddings(rows) }
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
