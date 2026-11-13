package ru.tinyops.turboist.nativeapp.labelsuggest

import android.app.Application
import androidx.room.Room
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import okhttp3.OkHttpClient
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import org.robolectric.RuntimeEnvironment
import org.robolectric.annotation.Config
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import java.io.File
import java.util.Collections
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * The index behind label suggestions, against a real replica: when it is
 * built, what it asks the model for, what it keeps, and what it forgets.
 *
 * The model is replaced by one that places a text on one of three axes by the
 * words in it, so which label a title should get is obvious from reading it.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class OnDeviceLabelSuggestionsTest {
    @get:Rule
    val folder = TemporaryFolder()

    private lateinit var db: TurboistDatabase
    private lateinit var models: File
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val switchedOn = MutableStateFlow(true)
    private val embedder = TopicEmbedder()

    private val now = System.currentTimeMillis()
    private var money = 0L
    private var health = 0L

    @Before
    fun setUp() {
        db = Room.inMemoryDatabaseBuilder(RuntimeEnvironment.getApplication(), TurboistDatabase::class.java).build()
        models = folder.newFolder("models")
        File(models, EmbeddingModel.FILE_NAME).writeBytes(ByteArray(MODEL_BYTES))
        runBlocking {
            money = db.labels().insert(label("деньги"))
            health = db.labels().insert(label("здоровье"))
            task("Оплатить хостинг", money)
            task("Оплатить налог", money)
            task("Записаться к врачу", health)
        }
    }

    @After
    fun tearDown() {
        scope.cancel()
        db.close()
    }

    private fun suggestions(embedder: TextEmbedder = this.embedder) =
        OnDeviceLabelSuggestions(
            database = db,
            switchedOn = switchedOn,
            model =
                ModelStore(
                    directory = models,
                    scope = scope,
                    client = OkHttpClient(),
                    source = ModelSource("http://localhost/unused", MODEL_BYTES.toLong(), "unused"),
                    freeBytes = { Long.MAX_VALUE },
                ),
            embedder = embedder,
            scope = scope,
            rebuildDelayMillis = 10,
        )

    @Test
    fun `a title like earlier ones is offered their label`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }

            settle(suggestions, learned = 3)

            withTimeout(TIMEOUT) { suggestions.available.first { it } }
            assertEquals(listOf(money), suggestions.suggest(capture("Оплатить интернет")))
            assertEquals(listOf(health), suggestions.suggest(capture("Сходить к врачу")))
        }

    @Test
    fun `a label already on the task is never offered`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 3)

            assertEquals(emptyList(), suggestions.suggest(capture("Оплатить интернет", exclude = setOf(money))))
        }

    @Test
    fun `nothing is embedded while suggestions are off`() =
        runBlocking {
            switchedOn.value = false
            val suggestions = suggestions().also { it.start() }
            delay(200)

            assertTrue(embedder.embedded.isEmpty())
            assertFalse(suggestions.available.value)
            assertEquals(emptyList(), suggestions.suggest(capture("Оплатить интернет")))
        }

    @Test
    fun `a second start reads every vector from the cache instead of the model`() =
        runBlocking {
            suggestions().also { it.start() }.let { settle(it, learned = 3) }
            awaitCached(count = 5)

            val again = TopicEmbedder()
            val restarted = suggestions(again).also { it.start() }
            settle(restarted, learned = 3)

            assertTrue(again.embedded.isEmpty(), "embedded again: ${again.embedded}")
            assertEquals(listOf(money), restarted.suggest(capture("Оплатить интернет")))
        }

    @Test
    fun `a new task sends only its own title to the model`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 3)
            embedder.embedded.clear()

            task("Купить витамины", health)
            settle(suggestions, learned = 4)

            assertEquals(listOf("task: classification | query: Купить витамины"), embedder.embedded.toList())
        }

    @Test
    fun `a deleted task's vector is dropped from the cache`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 3)
            awaitCached(count = 5)

            db.tasks().deleteByLocalIds(
                listOf(db.labelSuggestions().tasks().first { it.title == "Оплатить налог" }.localId),
            )

            settle(suggestions, learned = 2)
            awaitCached(count = 4)
        }

    @Test
    fun `turning suggestions off drops the index and closes the engine`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 3)

            switchedOn.value = false

            withTimeout(TIMEOUT) { suggestions.available.first { !it } }
            withTimeout(TIMEOUT) { suggestions.progress.first { it == IndexProgress() } }
            assertEquals(emptyList(), suggestions.suggest(capture("Оплатить интернет")))
            assertTrue(embedder.releases > 0)
        }

    @Test
    fun `a pass cut short keeps every batch it finished`() =
        runBlocking {
            // Two labels and 23 titles: more than one batch.
            repeat(20) { task("Оплатить счёт $it", money) }
            val gate = CompletableDeferred<Unit>()
            val stalling = TopicEmbedder(stallOnCall = 2, gate = gate)
            val suggestions = suggestions(stalling).also { it.start() }
            withTimeout(TIMEOUT) { stalling.stalled.await() }

            switchedOn.value = false
            withTimeout(TIMEOUT) { suggestions.available.first { !it } }

            // The first batch was stored even though the pass never finished.
            awaitCached(count = BATCH)
        }

    private fun capture(
        title: String,
        exclude: Set<Long> = emptySet(),
    ) = SuggestionRequest(title = title, exclude = exclude, surface = SuggestionSurface.CAPTURE)

    private suspend fun settle(
        suggestions: OnDeviceLabelSuggestions,
        learned: Int,
    ) {
        withTimeout(TIMEOUT) { suggestions.progress.first { it.learned == learned && it.total == learned } }
    }

    private suspend fun awaitCached(count: Int) {
        withTimeout(TIMEOUT) {
            while (db.textEmbeddings().embeddingKeys().size != count) delay(10)
        }
    }

    private suspend fun task(
        title: String,
        vararg labels: Long,
    ) {
        val id = db.tasks().insert(TaskRow(title = title, createdAt = now, updatedAt = now))
        db.tasks().setLabels(id, labels.toList(), taggedAt = now)
    }

    private fun label(name: String) = LabelRow(name = name, createdAt = now, updatedAt = now)

    /**
     * Money words point one way, health words another, anything else a third.
     * Records every text it was asked about; can be told to hang on one call.
     */
    private class TopicEmbedder(
        private val stallOnCall: Int = 0,
        private val gate: CompletableDeferred<Unit>? = null,
    ) : TextEmbedder {
        val embedded: MutableList<String> = Collections.synchronizedList(mutableListOf())
        val stalled = CompletableDeferred<Unit>()

        @Volatile var releases = 0
        private var calls = 0

        override suspend fun embed(texts: List<String>): List<FloatArray?> {
            calls++
            if (calls == stallOnCall) {
                stalled.complete(Unit)
                gate?.await()
            }
            embedded += texts
            return texts.map { text ->
                when {
                    MONEY.any { it in text } -> floatArrayOf(1f, 0f, 0f)
                    HEALTH.any { it in text } -> floatArrayOf(0f, 1f, 0f)
                    else -> floatArrayOf(0f, 0f, 1f)
                }
            }
        }

        override suspend fun release() {
            releases++
        }

        companion object {
            val MONEY = listOf("Оплатить", "деньги")
            val HEALTH = listOf("врач", "здоровье", "витамин")
        }
    }

    private companion object {
        const val MODEL_BYTES = 4
        const val TIMEOUT = 10_000L

        /** How many texts the index sends the model at once. */
        const val BATCH = 16
    }
}
