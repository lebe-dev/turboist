package ru.tinyops.turboist.nativeapp.labelsuggest

import android.app.Application
import androidx.room.Room
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.job
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
import ru.tinyops.turboist.core.database.entity.ContextRow
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.ProjectRow
import ru.tinyops.turboist.core.database.entity.TaskRow
import ru.tinyops.turboist.core.model.ProjectStatus
import java.io.File
import java.util.Collections
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

/**
 * The index behind label and project suggestions, against a real replica: when
 * it is built, what it asks the model for, what it keeps, what it forgets, and
 * which kind answers while only one of them is switched on.
 *
 * The model is replaced by one that places a text on one of three axes by the
 * words in it, so which label a title should get is obvious from reading it.
 */
@RunWith(RobolectricTestRunner::class)
@Config(sdk = [34], application = Application::class)
class OnDeviceSuggestionsTest {
    @get:Rule
    val folder = TemporaryFolder()

    private lateinit var db: TurboistDatabase
    private lateinit var models: File
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private val switchedOn = MutableStateFlow(true)
    private val projectsOn = MutableStateFlow(false)
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
        // A rebuild may be inside a blocking query; closing the database under it
        // would throw into whichever test runs next.
        runBlocking { scope.coroutineContext.job.cancelAndJoin() }
        db.close()
    }

    private fun suggestions(embedder: TextEmbedder = this.embedder) =
        OnDeviceSuggestions(
            database = db,
            switches = combine(switchedOn, projectsOn, ::SuggestionSwitches),
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

            withTimeout(TIMEOUT) { suggestions.labels.available.first { it } }
            assertEquals(listOf(money), suggestions.labels.suggest(capture("Оплатить интернет")))
            assertEquals(listOf(health), suggestions.labels.suggest(capture("Сходить к врачу")))
        }

    @Test
    fun `a label already on the task is never offered`() =
        runBlocking {
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 3)

            assertEquals(emptyList(), suggestions.labels.suggest(capture("Оплатить интернет", exclude = setOf(money))))
        }

    @Test
    fun `nothing is embedded while suggestions are off`() =
        runBlocking {
            switchedOn.value = false
            val suggestions = suggestions().also { it.start() }
            delay(200)

            assertTrue(embedder.embedded.isEmpty())
            assertFalse(suggestions.labels.available.value)
            assertEquals(emptyList(), suggestions.labels.suggest(capture("Оплатить интернет")))
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
            assertEquals(listOf(money), restarted.labels.suggest(capture("Оплатить интернет")))
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

            withTimeout(TIMEOUT) { suggestions.labels.available.first { !it } }
            withTimeout(TIMEOUT) { suggestions.progress.first { it == IndexProgress() } }
            assertEquals(emptyList(), suggestions.labels.suggest(capture("Оплатить интернет")))
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
            withTimeout(TIMEOUT) { suggestions.labels.available.first { !it } }

            // The first batch was stored even though the pass never finished.
            awaitCached(count = BATCH)
        }

    @Test
    fun `a title like earlier ones is offered their project`() =
        runBlocking {
            switchedOn.value = false
            projectsOn.value = true
            val bills = project("Счета")
            val clinic = project("Поликлиника")
            task("Оплатить свет", money, projectLocalId = bills)
            task("Оплатить воду", money, projectLocalId = bills)
            task("Купить витамины", health, projectLocalId = clinic)
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 6)

            withTimeout(TIMEOUT) { suggestions.projects.available.first { it } }
            assertEquals(listOf(bills), suggestions.projects.suggest(ProjectSuggestionRequest("Оплатить интернет")))
            assertEquals(listOf(clinic), suggestions.projects.suggest(ProjectSuggestionRequest("Сходить к врачу")))
            // Labels stay off while only projects are on, though the index is shared.
            assertFalse(suggestions.labels.available.value)
            assertEquals(emptyList(), suggestions.labels.suggest(capture("Оплатить интернет")))
        }

    @Test
    fun `a project that is no longer open is not offered`() =
        runBlocking {
            projectsOn.value = true
            val bills = project("Счета", status = ProjectStatus.ARCHIVED)
            task("Оплатить свет", money, projectLocalId = bills)
            task("Оплатить воду", money, projectLocalId = bills)
            project("Поликлиника")
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 5)

            assertEquals(emptyList(), suggestions.projects.suggest(ProjectSuggestionRequest("Оплатить интернет")))
        }

    @Test
    fun `projects stay quiet while their switch is off`() =
        runBlocking {
            val bills = project("Счета")
            task("Оплатить свет", money, projectLocalId = bills)
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 4)

            assertFalse(suggestions.projects.available.value)
            assertEquals(emptyList(), suggestions.projects.suggest(ProjectSuggestionRequest("Оплатить интернет")))
            assertEquals(listOf(money), suggestions.labels.suggest(capture("Оплатить интернет")))
        }

    @Test
    fun `labels and projects for one title cost the model one question`() =
        runBlocking {
            projectsOn.value = true
            val bills = project("Счета")
            task("Оплатить свет", money, projectLocalId = bills)
            val suggestions = suggestions().also { it.start() }
            settle(suggestions, learned = 4)
            embedder.embedded.clear()

            suggestions.labels.suggest(capture("Оплатить интернет"))
            suggestions.projects.suggest(ProjectSuggestionRequest("Оплатить интернет"))

            assertEquals(listOf("task: classification | query: Оплатить интернет"), embedder.embedded.toList())
        }

    private fun capture(
        title: String,
        exclude: Set<Long> = emptySet(),
    ) = SuggestionRequest(title = title, exclude = exclude, surface = SuggestionSurface.CAPTURE)

    private suspend fun settle(
        suggestions: OnDeviceSuggestions,
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
        projectLocalId: Long? = null,
    ) {
        val id =
            db.tasks().insert(
                TaskRow(title = title, projectLocalId = projectLocalId, createdAt = now, updatedAt = now),
            )
        db.tasks().setLabels(id, labels.toList(), taggedAt = now)
    }

    private fun label(name: String) = LabelRow(name = name, createdAt = now, updatedAt = now)

    private suspend fun project(
        title: String,
        status: ProjectStatus = ProjectStatus.OPEN,
    ): Long {
        val context = contextId ?: db.contexts().insert(ContextRow(name = "Дом", createdAt = now, updatedAt = now))
        contextId = context
        return db.projects().insert(
            ProjectRow(contextLocalId = context, title = title, status = status, createdAt = now, updatedAt = now),
        )
    }

    private var contextId: Long? = null

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
