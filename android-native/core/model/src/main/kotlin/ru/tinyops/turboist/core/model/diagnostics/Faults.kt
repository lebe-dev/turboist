package ru.tinyops.turboist.core.model.diagnostics

import java.util.concurrent.CancellationException

/** How much a fault matters to whoever reads the report. */
enum class FaultSeverity {
    /** Something the app handled, but which should not normally happen. */
    WARNING,

    /** A defect, or work the user did that may have been lost. */
    ERROR,
}

/**
 * One handled failure worth a developer's attention.
 *
 * A report leaves the device, so nothing in it may come from the user's data.
 *
 * @property area the part of the app it happened in.
 * @property message a fixed sentence written in the code; never built from data.
 * @property cause the exception behind it, if any.
 * @property tags short values the code itself enumerates, such as an error code
 *   or an operation kind; never titles, descriptions, tokens or addresses.
 */
data class Fault(
    val area: String,
    val message: String,
    val cause: Throwable?,
    val severity: FaultSeverity,
    val tags: Map<String, String>,
)

/** Where faults go. The app supplies one; the modules below it only know this shape. */
fun interface FaultSink {
    fun accept(fault: Fault)
}

/**
 * The one place handled failures are reported to.
 *
 * It is a process-wide object rather than a constructor dependency because it
 * has to be reachable from every layer, including plain functions, and has to
 * exist before the dependency graph does. With no sink installed — a test, a
 * build without reporting — a report is simply dropped.
 */
object Faults {
    @Volatile
    private var sink: FaultSink? = null

    /** Replaces the sink; `null` removes it. */
    fun install(replacement: FaultSink?) {
        sink = replacement
    }

    /**
     * Reports one failure.
     *
     * Cancellation is ignored, because several callers catch a broad
     * [RuntimeException] that it also falls under, and a screen going away is not
     * a failure. Whatever the sink does, the caller is unaffected: the code that
     * is reporting a failure is already handling one.
     */
    @Suppress("TooGenericExceptionCaught")
    fun report(
        area: String,
        message: String,
        cause: Throwable? = null,
        severity: FaultSeverity = FaultSeverity.ERROR,
        tags: Map<String, String> = emptyMap(),
    ) {
        if (cause is CancellationException) return
        try {
            sink?.accept(Fault(area, message, cause, severity, tags))
        } catch (_: RuntimeException) {
            // There is nowhere left to report this to.
        }
    }
}
