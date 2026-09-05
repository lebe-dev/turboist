package ru.tinyops.turboist.nativeapp.unsent

import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.sync.write.OutboxOpKind
import ru.tinyops.turboist.nativeapp.unsent.ui.labelFor
import ru.tinyops.turboist.nativeapp.unsent.ui.reasonFor
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNotEquals
import kotlin.test.assertTrue

/**
 * That every change a person can end up looking at has words for it.
 *
 * Any write the API offers can be refused and land on this list, so a gap here
 * shows up as a row saying nothing at all about work the user has lost. The
 * catalog is walked rather than sampled, which is what makes an op added
 * tomorrow fail here instead of on a phone.
 */
class UnsentWordingTest {
    @Test
    fun `every queued write has a name a person can read`() {
        OutboxOpKind.entries.forEach { kind ->
            assertNotEquals(0, labelFor(kind), "no wording for $kind")
        }
    }

    @Test
    fun `a write from a build this one does not know is still named`() {
        // It is shown rather than hidden: a change the app cannot describe is
        // exactly the kind that must not vanish without the user being told.
        assertNotEquals(0, labelFor(null))
    }

    @Test
    fun `no two writes share one name`() {
        // Two rows reading identically would be indistinguishable in a list whose
        // whole job is to say which change was lost. The two the web client also
        // queues are named in its words, which nothing else here uses.
        val named = OutboxOpKind.entries.map(::labelFor)

        assertEquals(named.size, named.toSet().size, "two writes are named by the same resource")
    }

    @Test
    fun `every refusal has a sentence`() {
        UnsentReason.entries.forEach { reason ->
            assertNotEquals(0, reasonFor(reason), "no wording for $reason")
        }
    }

    @Test
    fun `the refusals a user can act on are told apart`() {
        assertEquals(UnsentReason.BLOCKED, UnsentReason.of(ApiErrorCodes.TASK_BLOCKED))
        assertEquals(UnsentReason.LIMIT_REACHED, UnsentReason.of(ApiErrorCodes.LIMIT_EXCEEDED))
        assertEquals(UnsentReason.TROIKI_SLOT_FULL, UnsentReason.of(ApiErrorCodes.TROIKI_SLOT_FULL))
        assertEquals(UnsentReason.PLACEMENT_REFUSED, UnsentReason.of(ApiErrorCodes.FORBIDDEN_PLACEMENT))
    }

    @Test
    fun `a row the server no longer has reads the same however the refusal was phrased`() {
        // The drainer raises one of these itself when it finds the target gone
        // before sending; the server says the other when it is asked anyway.
        assertEquals(UnsentReason.TARGET_GONE, UnsentReason.of(ApiErrorCodes.TARGET_GONE))
        assertEquals(UnsentReason.TARGET_GONE, UnsentReason.of(ApiErrorCodes.NOT_FOUND))
    }

    @Test
    fun `a code this build has never heard of still reads as a refusal`() {
        // The pile is read long after the refusal, possibly by a build older than
        // the server that produced it. A list that cannot be rendered is worse
        // than one row that reads vaguely.
        assertEquals(UnsentReason.UNKNOWN, UnsentReason.of("a_code_from_the_future"))
        assertTrue(UnsentReason.of("") == UnsentReason.UNKNOWN)
    }
}
