package ru.tinyops.turboist.nativeapp.calendar

import ru.tinyops.turboist.core.database.sync.ReplicaEntityKind
import ru.tinyops.turboist.core.sync.write.OpNames
import kotlin.test.Test
import kotlin.test.assertTrue

/**
 * The calendar is outside the sync contract, and stays there.
 *
 * This is the one property of the feature that cannot be seen by looking at a
 * screen, so it is asserted rather than assumed. Appointments belong to a system
 * this app does not own: they are never created, changed or deleted here, so
 * they must never enter the copy of the user's data that a catch-up rewrites,
 * and never enter the queue of changes waiting to be sent. Both would be
 * meaningless at best — there is nothing to send — and at worst would let a
 * catch-up or a re-copy decide the fate of somebody else's diary.
 *
 * The guard is structural on purpose. The behaviour is already covered by the
 * repository's own checks, which run it against nothing but a scripted server and
 * an in-memory copy; what those cannot show is that a later change will not
 * quietly wire the two together.
 */
class CalendarStaysOutOfSyncTest {
    private val syncOwnedPackages =
        listOf(
            "ru.tinyops.turboist.core.database",
            "ru.tinyops.turboist.core.sync",
        )

    @Test
    fun `nothing in the calendar layer holds a piece of the replica or of the queue`() {
        val layer = listOf(CalendarRepository::class.java, DataStoreCalendarCache::class.java)
        for (type in layer) {
            val held = type.declaredFields.map { it.type.name }
            for (name in held) {
                assertTrue(
                    syncOwnedPackages.none { name.startsWith(it) },
                    "${type.simpleName} holds $name, which belongs to the replica or the outbox",
                )
            }
        }
    }

    @Test
    fun `the calendar is not one of the records a catch-up applies`() {
        assertTrue(
            ReplicaEntityKind.entries.none { it.stored.contains("calendar", ignoreCase = true) },
            "a calendar record has no place among the entities the change history reports",
        )
    }

    @Test
    fun `there is no queued write that would send a calendar anywhere`() {
        val ops =
            OpNames::class.java.declaredFields
                .filter { it.type == String::class.java }
                .map { field ->
                    field.isAccessible = true
                    field.get(OpNames) as String
                }
        assertTrue(ops.isNotEmpty(), "the catalogue of writes should not be empty; the check would prove nothing")
        assertTrue(
            ops.none { it.contains("calendar", ignoreCase = true) },
            "the calendar is read-only here, so no write of one can exist to be queued",
        )
    }
}
