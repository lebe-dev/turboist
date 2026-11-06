package ru.tinyops.turboist.core.model.diagnostics

import org.junit.After
import org.junit.Test
import java.util.concurrent.CancellationException
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class FaultsTest {
    private val received = mutableListOf<Fault>()

    @After
    fun removeSink() = Faults.install(null)

    @Test
    fun `a report reaches the installed sink`() {
        Faults.install(received::add)
        val cause = IllegalStateException("boom")

        Faults.report("sync", "It broke", cause, FaultSeverity.WARNING, mapOf("code" to "x"))

        assertEquals(listOf(Fault("sync", "It broke", cause, FaultSeverity.WARNING, mapOf("code" to "x"))), received)
    }

    @Test
    fun `a report with no sink is dropped`() {
        Faults.report("sync", "Nobody is listening")
    }

    @Test
    fun `cancellation is not a fault`() {
        Faults.install(received::add)

        Faults.report("tasks", "A write failed", CancellationException("the screen went away"))

        assertTrue(received.isEmpty())
    }

    @Test
    fun `a sink that fails does not affect the caller`() {
        Faults.install { error("the sink is broken") }

        Faults.report("sync", "It broke")
    }
}
