package ru.tinyops.turboist.core.sync.write

/**
 * Where the columns of a board end up after one of them is moved.
 *
 * This is the server's own rule, restated so the device can apply it before the
 * request is sent. The server does not move a single column: it takes the board
 * in its current order, lifts the moved column out, drops it back in at the
 * requested slot and then renumbers *every* column to `0..n-1`. Anything less
 * than that on the device would show a board that disagrees with the one the
 * next pull brings back, and the columns would visibly jump a second time.
 *
 * Two details are part of the rule rather than incidental:
 *
 * - **The target is clamped to the board.** A position past the last column
 *   means "put it last", and a negative one means "put it first". Neither is
 *   an error: they are what a drag past either end of the board means.
 * - **The order it starts from is position, then the row's own id.** Positions
 *   can tie — a column created on this device before the queue drained has no
 *   position from the server yet — and the id is what settles the tie on both
 *   sides.
 *
 * [ids] is the board as it stands, already in drawing order. The answer is the
 * same ids in their new order; a column's new position is its index.
 */
fun boardAfterMove(
    ids: List<Long>,
    movedId: Long,
    newPosition: Int,
): List<Long> {
    if (!ids.contains(movedId)) return ids
    val target = newPosition.coerceIn(0, ids.size - 1)
    val rest = ids.filter { it != movedId }
    return rest.subList(0, target) + movedId + rest.subList(target, rest.size)
}
