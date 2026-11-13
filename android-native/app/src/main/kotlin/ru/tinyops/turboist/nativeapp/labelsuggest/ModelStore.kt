package ru.tinyops.turboist.nativeapp.labelsuggest

import android.content.Context
import android.os.StatFs
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.ensureActive
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import ru.tinyops.turboist.core.model.diagnostics.Faults
import ru.tinyops.turboist.nativeapp.di.ApplicationScope
import java.io.File
import java.io.FileOutputStream
import java.io.IOException
import java.io.InputStream
import java.io.OutputStream
import java.security.MessageDigest
import java.util.concurrent.TimeUnit
import javax.inject.Inject
import javax.inject.Singleton
import kotlin.coroutines.coroutineContext

/** Where the model file stands on this device. */
sealed interface ModelState {
    /** Not here. [downloadedBytes] is how much of an interrupted download is kept for resuming. */
    data class Absent(
        val downloadedBytes: Long = 0,
    ) : ModelState

    data class Downloading(
        val downloadedBytes: Long,
        val totalBytes: Long,
    ) : ModelState

    /** On the device, checked, ready to open. */
    data object Ready : ModelState

    data class Failed(
        val reason: ModelFailure,
        val downloadedBytes: Long = 0,
    ) : ModelState
}

enum class ModelFailure {
    /** The server could not be reached or stopped answering; trying again resumes. */
    NETWORK,

    /** Not enough free space for the file. */
    STORAGE,

    /** What arrived is not the expected file; it was thrown away. */
    CORRUPT,
}

/**
 * The model file: whether it is here, getting it, and throwing it away.
 *
 * It lives in the app's no-backup storage — a cloud backup of a file anyone can
 * fetch again would only cost the user their backup quota — and is downloaded
 * by the process itself while the app runs, picking up where it stopped if it
 * is interrupted, rather than by a background job: the user has just asked for
 * it on the settings screen and is watching it arrive.
 */
@Singleton
class ModelStore internal constructor(
    private val directory: File,
    private val scope: CoroutineScope,
    private val client: OkHttpClient,
    private val source: ModelSource = ModelSource(),
    private val freeBytes: (File) -> Long = { StatFs(it.path).availableBytes },
) {
    @Inject
    constructor(
        @ApplicationContext context: Context,
        @ApplicationScope scope: CoroutineScope,
    ) : this(
        directory = File(context.noBackupFilesDir, "models"),
        scope = scope,
        client =
            OkHttpClient.Builder()
                .connectTimeout(CONNECT_TIMEOUT_SECONDS, TimeUnit.SECONDS)
                .readTimeout(READ_TIMEOUT_SECONDS, TimeUnit.SECONDS)
                .build(),
    )

    private val expectedSize: Long get() = source.sizeBytes

    /** The file the engine opens. Only meaningful while the state is [ModelState.Ready]. */
    val file: File get() = File(directory, EmbeddingModel.FILE_NAME)

    private val partial: File get() = File(directory, EmbeddingModel.FILE_NAME + ".part")

    private val current = MutableStateFlow(initialState())
    val state: StateFlow<ModelState> = current.asStateFlow()

    private var job: Job? = null

    /** Starts fetching the model, or carries on with an interrupted download. Does nothing while one runs. */
    @Synchronized
    fun download() {
        if (job?.isActive == true || current.value == ModelState.Ready) return
        job = scope.launch(Dispatchers.IO) { fetch() }
    }

    /** Stops a download, keeping what arrived so far. */
    @Synchronized
    fun cancel() {
        job?.cancel()
        job = null
        current.value = ModelState.Absent(partial.length())
    }

    /** Removes the model and any partial download. */
    suspend fun delete() {
        synchronized(this) {
            job?.cancel()
            job = null
        }
        withContext(Dispatchers.IO) {
            file.delete()
            partial.delete()
        }
        current.value = ModelState.Absent()
    }

    private fun initialState(): ModelState =
        when {
            // The hash was checked when the file was renamed into place, and only a
            // checked file is ever renamed; its size is enough to trust it here.
            file.length() == expectedSize -> ModelState.Ready
            else -> ModelState.Absent(partial.length())
        }

    @Suppress("TooGenericExceptionCaught")
    private suspend fun fetch() {
        try {
            directory.mkdirs()
            // Longer than the file can be means it is not the file: start over.
            if (partial.length() > expectedSize) partial.delete()
            if (!hasRoomFor(expectedSize - partial.length())) {
                current.value = ModelState.Failed(ModelFailure.STORAGE, partial.length())
                return
            }
            if (partial.length() < expectedSize) transfer()
            finish()
        } catch (cancelled: CancellationException) {
            throw cancelled
        } catch (_: IOException) {
            current.value = ModelState.Failed(ModelFailure.NETWORK, partial.length())
        } catch (failure: RuntimeException) {
            Faults.report("labelsuggest", "Downloading the label suggestion model failed", failure)
            current.value = ModelState.Failed(ModelFailure.NETWORK, partial.length())
        }
    }

    private suspend fun transfer() {
        val have = partial.length()
        val request =
            Request.Builder().url(source.url).apply { if (have > 0) header("Range", "bytes=$have-") }.build()
        client.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw IOException("The model server answered ${response.code}")
            // A server that ignores the range sends the whole file again from the start.
            val resuming = response.code == HTTP_PARTIAL_CONTENT
            val written = if (resuming) have else 0L
            current.value = ModelState.Downloading(written, expectedSize)
            FileOutputStream(partial, resuming).use { out ->
                response.body.byteStream().use { input -> copy(input, out, written) }
            }
        }
    }

    /** Copies the body onto the partial file, saying how far it has got every half megabyte. */
    private suspend fun copy(
        input: InputStream,
        out: OutputStream,
        start: Long,
    ) {
        val buffer = ByteArray(BUFFER_BYTES)
        var written = start
        var lastReported = start
        while (true) {
            coroutineContext.ensureActive()
            val read = input.read(buffer)
            if (read < 0) return
            out.write(buffer, 0, read)
            written += read
            if (written - lastReported >= PROGRESS_STEP_BYTES) {
                current.value = ModelState.Downloading(written, expectedSize)
                lastReported = written
            }
        }
    }

    private fun finish() {
        if (partial.length() != expectedSize || sha256(partial) != source.sha256) {
            partial.delete()
            current.value = ModelState.Failed(ModelFailure.CORRUPT)
            return
        }
        if (!partial.renameTo(file)) throw IOException("The model could not be moved into place")
        current.value = ModelState.Ready
    }

    private fun hasRoomFor(bytes: Long): Boolean = freeBytes(directory) > bytes + SPARE_BYTES

    private companion object {
        const val CONNECT_TIMEOUT_SECONDS = 30L
        const val READ_TIMEOUT_SECONDS = 60L
        const val HTTP_PARTIAL_CONTENT = 206
        const val BUFFER_BYTES = 64 * 1024
        const val PROGRESS_STEP_BYTES = 512L * 1024
        const val SPARE_BYTES = 50L * 1024 * 1024

        fun sha256(file: File): String {
            val digest = MessageDigest.getInstance("SHA-256")
            file.inputStream().use { input ->
                val buffer = ByteArray(BUFFER_BYTES)
                while (true) {
                    val read = input.read(buffer)
                    if (read < 0) break
                    digest.update(buffer, 0, read)
                }
            }
            return digest.digest().joinToString("") { "%02x".format(it) }
        }
    }
}

/** Where the model is fetched from, and what it has to be once it arrives. */
data class ModelSource(
    val url: String = EmbeddingModel.URL,
    val sizeBytes: Long = EmbeddingModel.SIZE_BYTES,
    val sha256: String = EmbeddingModel.SHA256,
)
