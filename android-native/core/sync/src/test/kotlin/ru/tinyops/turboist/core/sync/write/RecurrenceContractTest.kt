package ru.tinyops.turboist.core.sync.write

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Test
import org.junit.runner.RunWith
import org.robolectric.RobolectricTestRunner
import ru.tinyops.turboist.core.model.WireTime
import java.io.File
import java.time.ZoneId
import kotlin.test.assertTrue

/**
 * When a repeating task next falls due, against the answers the server gives.
 *
 * The device works this out for itself so that a task ticked off with no
 * connection shows its next run immediately. That only helps if the answer is
 * the one the server would have given: a date that appears and is silently
 * replaced after the next catch-up reads as a bug in the user's plan.
 *
 * So one list of worked examples lives beside the server's own tests, is written
 * by the server's implementation, and is answered here by this one. When a case
 * stops matching, exactly one of two things is true: this side is wrong, or the
 * advance genuinely changed — and a genuine change is made in the shared file
 * first, on both sides at once. Nothing here is ever "adjusted" alone.
 */
@RunWith(RobolectricTestRunner::class)
class RecurrenceContractTest {
    @Test
    fun `every worked example is answered the way the server answers it`() {
        val failures = mutableListOf<String>()
        for (example in workedExamples()) {
            val anchor = RecurrenceAdvancer.anchorFor(example.dueAt, example.completedAt)
            if (anchor != example.anchor) {
                failures += "${example.key}: anchored at ${WireTime.format(anchor)}, " +
                    "the contract says ${WireTime.format(example.anchor)}"
                continue
            }
            val outcome = RecurrenceAdvancer(example.zone).after(example.rule, example.dueAt, example.completedAt)
            val expected = example.next
            when {
                expected == null && outcome != RecurrenceOutcome.Ended ->
                    failures += "${example.key}: the series should have ended, and the device said $outcome"
                expected != null && outcome != RecurrenceOutcome.Next(expected) ->
                    failures += "${example.key}: the contract says ${WireTime.format(expected)}, " +
                        "and the device said $outcome"
            }
        }
        assertTrue(
            failures.isEmpty(),
            "these repeat rules no longer advance the way the server advances them:\n" + failures.joinToString("\n"),
        )
    }

    @Test
    fun `the shared file carries examples to answer`() {
        // A file that failed to load, or that lost its cases in an edit, would
        // make the check above pass by never asking anything.
        assertTrue(workedExamples().size >= MINIMUM_EXAMPLES, "the shared recurrence contract has too few cases")
    }

    /** One question from the shared file, in the device's own units. */
    private data class WorkedExample(
        val key: String,
        val rule: String,
        val zone: ZoneId,
        val dueAt: Long?,
        val completedAt: Long,
        val anchor: Long,
        val next: Long?,
    )

    private fun workedExamples(): List<WorkedExample> {
        val configured =
            System.getProperty("turboist.recurrenceContract.dir")
                ?: error("the build did not tell the tests where the shared recurrence examples are")
        val file = File(configured, "fixture.json")
        require(file.isFile) { "the shared recurrence contract is missing: $file" }
        val parsed = Json.parseToJsonElement(file.readText()) as JsonObject
        return parsed.getValue("cases").jsonArray.map { element ->
            val case = element as JsonObject

            fun text(name: String): String = case.getValue(name).jsonPrimitive.content

            fun moment(name: String): Long? = text(name).takeIf { it.isNotEmpty() }?.let(WireTime::parse)

            WorkedExample(
                key = text("key"),
                rule = text("rule"),
                zone = ZoneId.of(text("timezone")),
                dueAt = moment("dueAt"),
                completedAt = requireNotNull(moment("completedAt")) { "${text("key")}: no moment of completion" },
                anchor = requireNotNull(moment("anchor")) { "${text("key")}: no anchor" },
                next = moment("next"),
            )
        }
    }

    private companion object {
        /**
         * How many cases the file has to carry before this check counts as one.
         * Well below what is in it, so a deliberate addition never trips it while
         * an accidental emptying does.
         */
        const val MINIMUM_EXAMPLES: Int = 10
    }
}
