package ru.tinyops.turboist.nativeapp.diagnostics

import io.sentry.Breadcrumb
import io.sentry.SentryEvent
import io.sentry.SentryLevel
import io.sentry.protocol.Request
import io.sentry.protocol.SentryException
import io.sentry.protocol.User
import org.junit.Test
import ru.tinyops.turboist.core.model.diagnostics.Fault
import ru.tinyops.turboist.core.model.diagnostics.FaultSeverity
import kotlin.test.assertEquals
import kotlin.test.assertNull
import kotlin.test.assertSame

class ErrorReportingTest {
    @Test
    fun `nothing that could hold user data leaves the device`() {
        val event =
            SentryEvent().apply {
                exceptions =
                    listOf(
                        SentryException().apply {
                            type = "SerializationException"
                            value = "Unexpected JSON near {\"title\":\"See the doctor\"}"
                        },
                    )
                user = User().apply { username = "me" }
                request = Request().apply { url = "https://my.server/api" }
                serverName = "My Phone"
                breadcrumbs =
                    listOf(
                        Breadcrumb("Task: see the doctor").apply { category = "console" },
                        Breadcrumb("created").apply { category = "ui.lifecycle" },
                    )
            }

        val sent = scrubbed(event)

        assertNull(sent.exceptions!!.single().value)
        assertEquals("SerializationException", sent.exceptions!!.single().type)
        assertNull(sent.user)
        assertNull(sent.request)
        assertNull(sent.serverName)
        assertEquals(listOf("ui.lifecycle"), sent.breadcrumbs!!.map { it.category })
    }

    @Test
    fun `a fault becomes an event with its sentence, level and tags`() {
        val cause = IllegalStateException("boom")

        val event =
            eventOf(Fault("sync", "It broke", cause, FaultSeverity.WARNING, mapOf("code" to "internal_error")))

        assertEquals("It broke", event.message?.message)
        assertEquals(SentryLevel.WARNING, event.level)
        assertEquals("sync", event.getTag("area"))
        assertEquals("internal_error", event.getTag("code"))
        assertSame(cause, event.throwable)
    }

    @Test
    fun `a fault without an exception is still an event`() {
        val event = eventOf(Fault("sync", "It broke", null, FaultSeverity.ERROR, emptyMap()))

        assertEquals(SentryLevel.ERROR, event.level)
        assertNull(event.throwable)
    }
}
