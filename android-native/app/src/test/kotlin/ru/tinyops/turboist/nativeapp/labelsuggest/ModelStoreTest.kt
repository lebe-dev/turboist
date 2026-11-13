package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import mockwebserver3.Dispatcher
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import mockwebserver3.RecordedRequest
import okhttp3.OkHttpClient
import okio.Buffer
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.TimeUnit
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull
import kotlin.test.assertTrue

/** Fetching the model file: whole, resumed, refused and damaged. */
class ModelStoreTest {
    @get:Rule
    val folder = TemporaryFolder()

    private val server = MockWebServer()
    private val scope = CoroutineScope(SupervisorJob())
    private val content = ByteArray(200_000) { (it % 251).toByte() }
    private lateinit var directory: File

    @Before
    fun setUp() {
        server.start()
        directory = folder.newFolder("models")
    }

    @After
    fun tearDown() {
        scope.cancel()
        server.close()
    }

    private fun store(
        sha256: String = sha256(content),
        free: Long = Long.MAX_VALUE,
    ) = ModelStore(
        directory = directory,
        scope = scope,
        client = OkHttpClient(),
        source = ModelSource(server.url("/model").toString(), content.size.toLong(), sha256),
        freeBytes = { free },
    )

    private fun ModelStore.settle(): ModelState =
        runBlocking {
            withTimeout(10_000) { state.first { it == ModelState.Ready || it is ModelState.Failed } }
        }

    private fun body(bytes: ByteArray) = Buffer().write(bytes)

    @Test
    fun `a downloaded file that matches is moved into place`() {
        server.enqueue(MockResponse.Builder().code(200).body(body(content)).build())
        val store = store()

        store.download()

        assertEquals(ModelState.Ready, store.settle())
        assertContentEquals(content, store.file.readBytes())
        assertNull(server.takeRequest().headers["Range"])
    }

    @Test
    fun `an interrupted download carries on from where it stopped`() {
        val half = content.size / 2
        File(directory, EmbeddingModel.FILE_NAME + ".part").writeBytes(content.copyOf(half))
        server.enqueue(
            MockResponse.Builder().code(206).body(body(content.copyOfRange(half, content.size))).build(),
        )
        val store = store()
        assertEquals(ModelState.Absent(half.toLong()), store.state.value)

        store.download()

        assertEquals(ModelState.Ready, store.settle())
        assertEquals("bytes=$half-", server.takeRequest().headers["Range"])
        assertContentEquals(content, store.file.readBytes())
    }

    @Test
    fun `a server that ignores the range sends the file again from the start`() {
        File(directory, EmbeddingModel.FILE_NAME + ".part").writeBytes(content.copyOf(1_000))
        server.enqueue(MockResponse.Builder().code(200).body(body(content)).build())
        val store = store()

        store.download()

        assertEquals(ModelState.Ready, store.settle())
        assertContentEquals(content, store.file.readBytes())
    }

    @Test
    fun `a damaged file is thrown away rather than opened`() {
        server.enqueue(MockResponse.Builder().code(200).body(body(content)).build())
        val store = store(sha256 = "0".repeat(64))

        store.download()

        assertEquals(ModelState.Failed(ModelFailure.CORRUPT), store.settle())
        assertFalse(store.file.exists())
        assertFalse(File(directory, EmbeddingModel.FILE_NAME + ".part").exists())
    }

    @Test
    fun `a refusal keeps what arrived so a retry can resume`() {
        server.enqueue(MockResponse.Builder().code(503).build())
        val store = store()

        store.download()

        assertEquals(ModelState.Failed(ModelFailure.NETWORK), store.settle())
    }

    @Test
    fun `no download starts without room for the file`() {
        val store = store(free = 10)

        store.download()

        assertEquals(ModelState.Failed(ModelFailure.STORAGE), store.settle())
        assertEquals(0, server.requestCount)
    }

    @Test
    fun `a file already in place is ready at once, and deleting it removes it`() {
        File(directory, EmbeddingModel.FILE_NAME).writeBytes(content)
        val store = store()

        assertEquals(ModelState.Ready, store.state.value)

        runBlocking { store.delete() }

        assertEquals(ModelState.Absent(), store.state.value)
        assertFalse(store.file.exists())
    }

    @Test
    fun `stopping a download keeps what arrived, and starting again carries on`() {
        server.dispatcher = RangeDispatcher(throttled = true)
        val store = store()
        store.download()
        awaitPartial()

        store.cancel()
        // Long enough for a progress report racing the stop to have landed.
        runBlocking { delay(300) }

        val stopped = store.state.value
        assertTrue(stopped is ModelState.Absent && stopped.downloadedBytes > 0, "after stopping: $stopped")
        server.dispatcher = RangeDispatcher(throttled = false)
        store.download()
        assertEquals(ModelState.Ready, store.settle())
        assertContentEquals(content, store.file.readBytes())
    }

    @Test
    fun `deleting in the middle of a download leaves nothing behind`() {
        server.dispatcher = RangeDispatcher(throttled = true)
        val store = store()
        store.download()
        awaitPartial()

        runBlocking {
            store.delete()
            delay(300)
        }

        assertEquals(ModelState.Absent(), store.state.value)
        assertFalse(store.file.exists())
        assertFalse(File(directory, EmbeddingModel.FILE_NAME + ".part").exists())
    }

    /** Waits until part of the file has been written. */
    private fun awaitPartial() =
        runBlocking {
            withTimeout(10_000) {
                while (File(directory, EmbeddingModel.FILE_NAME + ".part").length() == 0L) delay(10)
            }
        }

    /** Answers a ranged request with the rest of the file, slowly if asked to. */
    private inner class RangeDispatcher(
        private val throttled: Boolean,
    ) : Dispatcher() {
        override fun dispatch(request: RecordedRequest): MockResponse {
            val from = request.headers["Range"]?.removePrefix("bytes=")?.removeSuffix("-")?.toInt() ?: 0
            return MockResponse.Builder()
                .code(if (from > 0) 206 else 200)
                .body(body(content.copyOfRange(from, content.size)))
                .apply { if (throttled) throttleBody(4_096, 50, TimeUnit.MILLISECONDS) }
                .build()
        }
    }

    private fun sha256(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
