package ru.tinyops.turboist.nativeapp.diagnostics

import android.content.Context
import android.util.Log
import io.sentry.Breadcrumb
import io.sentry.Sentry
import io.sentry.SentryEvent
import io.sentry.SentryLevel
import io.sentry.SentryOptions
import io.sentry.android.core.SentryAndroid
import io.sentry.android.core.SentryAndroidOptions
import io.sentry.protocol.Message
import ru.tinyops.turboist.core.model.diagnostics.Fault
import ru.tinyops.turboist.core.model.diagnostics.FaultSeverity
import ru.tinyops.turboist.core.model.diagnostics.Faults
import ru.tinyops.turboist.nativeapp.BuildConfig
import java.io.File

/**
 * Sends crashes and handled failures to the developer, once the user has said so.
 *
 * Off means off: the SDK is not running, nothing is collected, and whatever was
 * waiting on disk when the switch was turned off is deleted rather than sent
 * later. Only errors are sent — no performance traces, no session tracking, no
 * screenshots, no user. Each event also passes through [scrubbed] on its way out,
 * which is where the promise that no user data leaves the device is kept.
 */
object ErrorReporting {
    private const val TAG = "TurboistReporting"

    /** Whether this build was given anywhere to send reports. */
    val available: Boolean
        get() = BuildConfig.SENTRY_DSN.isNotBlank()

    /**
     * Sets reporting up for this process, before anything else in it has run.
     *
     * Called ahead of the dependency graph, so a crash while building it is caught
     * too, and so a fault reported at any later moment has somewhere to go.
     */
    fun start(context: Context) {
        if (!available) return
        Faults.install(::send)
        if (ErrorReportingConsent(context.applicationContext).allowed.value) open(context)
    }

    /** Brings the running process in line with a choice the user just made. */
    fun apply(
        context: Context,
        allowed: Boolean,
    ) {
        if (!available) return
        if (allowed) {
            open(context)
            return
        }
        Sentry.close()
        File(context.cacheDir, "sentry").deleteRecursively()
    }

    private fun open(context: Context) {
        try {
            SentryAndroid.init(context, ::configure)
        } catch (malformed: IllegalArgumentException) {
            // A build given a broken DSN should still run; it just cannot report.
            Log.w(TAG, "Error reporting could not start: the configured DSN is not valid", malformed)
        }
    }

    private fun configure(options: SentryAndroidOptions) {
        options.dsn = BuildConfig.SENTRY_DSN
        BuildConfig.SENTRY_ENVIRONMENT.takeIf(String::isNotBlank)?.let { options.environment = it }
        options.setTag("client", "android-native")
        options.isSendDefaultPii = false
        options.isAttachServerName = false
        options.isAttachScreenshot = false
        options.isAttachViewHierarchy = false
        options.isEnableAutoSessionTracking = false
        options.isEnableUserInteractionBreadcrumbs = false
        options.isEnableSystemEventBreadcrumbs = false
        options.isEnableNetworkEventBreadcrumbs = false
        options.isEnableAppComponentBreadcrumbs = false
        options.beforeSend = SentryOptions.BeforeSendCallback { event, _ -> scrubbed(event) }
        options.beforeBreadcrumb = SentryOptions.BeforeBreadcrumbCallback { crumb, _ -> crumb.takeIf(::isSafe) }
    }

    private fun send(fault: Fault) {
        if (Sentry.isEnabled()) Sentry.captureEvent(eventOf(fault))
    }
}

/** The breadcrumb categories that describe the app's lifecycle and nothing the user wrote. */
private val SAFE_BREADCRUMBS = setOf("app.lifecycle", "ui.lifecycle")

internal fun isSafe(crumb: Breadcrumb): Boolean = crumb.category in SAFE_BREADCRUMBS

/**
 * The event [fault] becomes.
 *
 * The message is a fixed sentence and the tags are values the code enumerates, so
 * grouping in the dashboard follows what went wrong rather than what it was about.
 */
internal fun eventOf(fault: Fault): SentryEvent {
    val event = fault.cause?.let(::SentryEvent) ?: SentryEvent()
    event.message = Message().apply { message = fault.message }
    event.level = if (fault.severity == FaultSeverity.ERROR) SentryLevel.ERROR else SentryLevel.WARNING
    event.setTag("area", fault.area)
    fault.tags.forEach(event::setTag)
    return event
}

/**
 * The event as it is allowed to leave the device.
 *
 * Exception types and stack frames stay; exception messages go, without trying to
 * tell the harmless ones apart. Messages here carry things like the server's
 * address, the server's wording, and fragments of whatever JSON failed to decode,
 * which can be a task's title. A rule that removes all of them cannot miss one.
 */
internal fun scrubbed(event: SentryEvent): SentryEvent {
    event.exceptions?.forEach { it.value = null }
    event.user = null
    event.request = null
    event.serverName = null
    event.breadcrumbs = event.breadcrumbs?.filter(::isSafe)
    return event
}
