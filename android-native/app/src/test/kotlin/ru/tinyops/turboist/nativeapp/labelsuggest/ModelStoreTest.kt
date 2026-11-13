package ru.tinyops.turboist.nativeapp.labelsuggest

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.runBlocking
import kotlinx.coroutines.withTimeout
import mockwebserver3.MockResponse
import mockwebserver3.MockWebServer
import okhttp3.OkHttpClient
import okio.Buffer
import org.junit.After
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File
import java.security.MessageDigest
import kotlin.test.assertContentEquals
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull

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

    private fun sha256(bytes: ByteArray): String =
        MessageDigest.getInstance("SHA-256").digest(bytes).joinToString("") { "%02x".format(it) }
}
