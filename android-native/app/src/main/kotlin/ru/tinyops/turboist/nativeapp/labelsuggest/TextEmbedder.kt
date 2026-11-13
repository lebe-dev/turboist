package ru.tinyops.turboist.nativeapp.labelsuggest

import android.content.Context
import com.google.ai.edge.litertlm.Backend
import com.google.ai.edge.litertlm.EmbeddingEngine
import com.google.ai.edge.litertlm.EmbeddingEngineConfig
import com.google.ai.edge.litertlm.EmbeddingOptions
import com.google.ai.edge.litertlm.InputData
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.ExperimentalCoroutinesApi
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import ru.tinyops.turboist.core.model.diagnostics.Faults
import ru.tinyops.turboist.core.model.labelsuggest.Vectors
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import javax.inject.Inject
import javax.inject.Singleton

/** Turns texts into embeddings. */
interface TextEmbedder {
    /**
     * One embedding per text, in order, already prepared for scoring; `null` for
     * a text that could not be embedded, and all `null` while there is no model.
     */
    suspend fun embed(texts: List<String>): List<FloatArray?>

    /** Gives back whatever the embedder holds open; the next [embed] opens it again. */
    suspend fun release() {}
}

/**
 * [TextEmbedder] on the LiteRT-LM engine, running on the phone's CPU.
 *
 * The engine holds a few hundred megabytes while it is open, so it is opened on
 * the first request and closed again once nothing has asked for a minute: a
 * capture sheet open for a few seconds should not keep that memory for the rest
 * of the day. Every call goes through one thread — the engine is not documented
 * as safe to share, and requests are short enough that queueing costs nothing.
 */
@Singleton
class LiteRtEmbedder
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
        private val model: ModelStore,
        @param:ApplicationScope private val scope: CoroutineScope,
    ) : TextEmbedder {
        @OptIn(ExperimentalCoroutinesApi::class)
        private val thread = Dispatchers.Default.limitedParallelism(1)

        private var engine: EmbeddingEngine? = null
        private var closing: Job? = null

        /**
         * Set once the engine has refused to open. A model that cannot be opened
         * will not open on the next try either, and the first indexing pass asks
         * a few hundred times in a row: each failure would be one more identical
         * report. Cleared by [release], which is what deleting or re-enabling
         * the model goes through.
         */
        private var unavailable = false

        override suspend fun embed(texts: List<String>): List<FloatArray?> {
            if (texts.isEmpty()) return emptyList()
            if (model.state.value != ModelState.Ready) return texts.map { null }
            return withContext(thread) {
                closing?.cancel()
                if (unavailable) return@withContext texts.map { null }
                val opened = engine ?: open() ?: return@withContext texts.map { null }
                val result = texts.chunked(BATCH).flatMap { batch -> embedBatch(opened, batch) }
                closing = scope.launch(thread) { closeWhenIdle() }
                result
            }
        }

        @Suppress("TooGenericExceptionCaught")
        private fun open(): EmbeddingEngine? =
            try {
                EmbeddingEngine(
                    EmbeddingEngineConfig(
                        modelPath = model.file.path,
                        backend = Backend.CPU(),
                        cacheDir = context.cacheDir.path,
                        maxInputLength = EmbeddingModel.MAX_INPUT_TOKENS,
                    ),
                ).also {
                    it.initialize()
                    engine = it
                }
            } catch (failure: RuntimeException) {
                Faults.report("labelsuggest", "The label suggestion model could not be opened", failure)
                unavailable = true
                null
            } catch (missing: LinkageError) {
                // The engine's native library is not built for this phone.
                Faults.report("labelsuggest", "The label suggestion engine is not available on this device", missing)
                unavailable = true
                null
            }

        /**
         * Embeds a batch at once, falling back to one text at a time if the batch
         * is refused, so one text the engine cannot take does not cost the others.
         */
        @Suppress("TooGenericExceptionCaught")
        private fun embedBatch(
            engine: EmbeddingEngine,
            texts: List<String>,
        ): List<FloatArray?> =
            try {
                engine.computeEmbeddingBatch(
                    texts.map { listOf(input(it)) },
                    OPTIONS,
                ).map { Vectors.prepare(it.embedding) }
            } catch (_: RuntimeException) {
                texts.map { text ->
                    try {
                        Vectors.prepare(engine.computeEmbedding(listOf(input(text)), OPTIONS).embedding)
                    } catch (_: RuntimeException) {
                        null
                    }
                }
            }

        /** Closes the engine now, for when the model is about to be deleted or suggestions are turned off. */
        override suspend fun release() {
            withContext(thread) {
                closing?.cancel()
                close()
                unavailable = false
            }
        }

        private suspend fun closeWhenIdle() {
            delay(IDLE_MILLIS)
            close()
        }

        private fun close() {
            engine?.close()
            engine = null
        }

        private companion object {
            const val BATCH = 16
            const val IDLE_MILLIS = 60_000L

            /** Unit length from the engine; [Vectors.prepare] then cuts and rescales. */
            val OPTIONS = EmbeddingOptions(normalize = true)

            fun input(text: String) = InputData.Text(text.take(EmbeddingModel.MAX_INPUT_CHARS))
        }
    }
