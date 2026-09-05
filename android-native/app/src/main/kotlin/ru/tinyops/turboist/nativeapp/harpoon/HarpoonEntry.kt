package ru.tinyops.turboist.nativeapp.harpoon

import ru.tinyops.turboist.core.model.HARPOON_SLOTS
import ru.tinyops.turboist.core.sync.write.HarpoonTarget

/**
 * One end of the jump pair, as this device holds it.
 *
 * Named by the id this device holds the row under, not the server's: a project
 * started on a plane can be jumped to that afternoon, hours before it has an id
 * anywhere else, and a reference recorded as "nothing yet" could never be
 * repaired. The request that carries it to the server is translated at the
 * moment it is sent, by which time the row it names does have an id.
 */
data class HarpoonEntry(
    val target: HarpoonTarget,
    val localId: Long,
)

/**
 * The pair with [entry] hooked on.
 *
 * Two slots, oldest out first, and hooking on something already in the pair
 * moves it to the newest end rather than adding it twice — which is the rule the
 * server applies to the same gesture. Repeating it here is what makes the pair
 * right at the moment of the tap instead of after the queue drains; both sides
 * then reach the same two entries, in the same order, from the same taps.
 */
fun withHarpooned(
    existing: List<HarpoonEntry>,
    entry: HarpoonEntry,
): List<HarpoonEntry> = (existing.filterNot { it == entry } + entry).takeLast(HARPOON_SLOTS)

/** The pair with [entry] unhooked. Unhooking something that is not in it changes nothing. */
fun withoutHarpooned(
    existing: List<HarpoonEntry>,
    entry: HarpoonEntry,
): List<HarpoonEntry> = existing.filterNot { it == entry }
