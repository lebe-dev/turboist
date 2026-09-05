package ru.tinyops.turboist.core.sync.drain

import android.util.Log
import androidx.room.withTransaction
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.database.entity.OutboxOpRow
import ru.tinyops.turboist.core.database.sync.OutboxState
import ru.tinyops.turboist.core.network.ApiErrorCodes
import ru.tinyops.turboist.core.network.ApiException
import ru.tinyops.turboist.core.sync.SyncMutex
import ru.tinyops.turboist.core.sync.write.OutboxOp
import ru.tinyops.turboist.core.sync.write.OutboxOpCodec
import ru.tinyops.turboist.core.sync.write.ReplicaServerIds
import ru.tinyops.turboist.core.sync.write.UnsentReference
import ru.tinyops.turboist.core.sync.write.WriteClock
import java.io.IOException

/**
 * Sends what the user changed while the server could not be reached.
 *
 * The queue is drained strictly in the order it was written, one write at a
 * time. That is not caution, it is the meaning of the queue: a task moved into a
 * project and then completed is a different outcome from a task completed and
 * then moved, and only the order the user acted in produces the one they saw.
 * So a write that cannot be sent stops the queue rather than being stepped over.
 *
 * Every request carries the write's own id as its idempotency key. The key was
 * minted when the write was queued, so an attempt whose answer was lost — a
 * dropped connection, a process killed between sending and hearing back — is
 * safe to make again: the server recognises the repeat and replays what it
 * already decided instead of doing the work twice. This is why a write is marked
 * as being in flight before it is sent and only removed once an answer arrives:
 * the worst case is one extra request, never one extra task.
 *
 * **What each kind of failure means.**
 *
 * - *Nothing reached the server.* The write keeps its place and the queue waits.
 *   Nothing is lost and nothing is reordered; the wait grows with each attempt
 *   so a phone with no signal is not spending its battery on the radio.
 * - *The server broke.* The request itself may be perfectly good, so it is
 *   retried like a lost connection — but only so many times. A write that a
 *   server keeps failing on would otherwise hold up every write behind it
 *   forever, so after a few attempts it is set aside.
 * - *The server refused.* It understood the request and said no, and it will say
 *   no to the identical request every time. The write leaves the queue at once
 *   and the queue carries on. Nothing is thrown away: the write is kept where
 *   the user can see what did not happen, together with the part of the answer
 *   that says what to do about it — for a completion refused on an open blocker,
 *   which tasks were in the way, since the wording of that refusal is the same
 *   sentence every time and the ids are in this one answer and nowhere else.
 * - *The credentials are the problem.* No amount of retrying helps until the
 *   user signs in again, so the drain stops with everything intact.
 *
 * Nothing here undoes the optimistic change a refused write made to the replica.
 * The catch-up that follows the drain carries the server's own version of those
 * rows, which is the authority; a hand-rolled rollback would be a second, worse
 * answer to the same question and the two would eventually disagree.
 *
 * Local ids are translated into the server's at the moment of sending rather
 * than written into the queued payloads. A write queued against a row that only
 * exists on this device has nothing else to name it by, and the order of the
 * queue guarantees the row's own creation is sent first — so by the time a write
 * that refers to it goes out, the answer to "what does the server call this" is
 * already recorded.
 */
class OutboxDrainer(
    private val db: TurboistDatabase,
    private val sender: OpSender,
    private val ids: ReplicaServerIds,
    private val mutex: SyncMutex,
    private val clock: WriteClock = WriteClock.System,
    private val backoff: DrainBackoff = DrainBackoff.Default,
    private val serverErrorLimit: Int = DEFAULT_SERVER_ERROR_LIMIT,
) {
    /**
     * How many passes in a row have ended with the queue waiting, which is what
     * the wait before the next one is measured from.
     *
     * It is kept in memory rather than in the row on purpose. What the row has to
     * survive a restart with is how often the *server* has broken on this write,
     * because that is what decides whether it can ever land. How long to wait is
     * a property of the moment, and a process that has just started is a moment
     * worth trying immediately.
     */
    private var consecutiveStalls: Int = 0

    /**
     * Sends everything waiting, or reports why it stopped.
     *
     * Holds the one lock the sync engine has, so a catch-up never runs beside a
     * send: a read that overlapped a send could write the server's pre-send state
     * over the very rows the user just changed.
     */
    suspend fun drain(): DrainResult = mutex.withExclusiveAccess { drainQueue() }

    @Suppress("ReturnCount")
    private suspend fun drainQueue(): DrainResult {
        var sent = 0
        var quarantined = 0
        while (true) {
            val row = db.outbox().head()
            if (row == null) {
                consecutiveStalls = 0
                if (sent > 0 || quarantined > 0) {
                    Log.i(DRAIN_LOG_TAG, "Sent $sent queued change(s); $quarantined were refused and set aside")
                }
                return DrainResult.Drained(sent, quarantined)
            }

            val op = readable(row)
            if (op == null) {
                setAside(row, ApiErrorCodes.WRITE_UNSENDABLE, UNREADABLE_MESSAGE, status = null)
                quarantined++
                continue
            }

            markInFlight(row)
            try {
                send(row, op)
                sent++
                consecutiveStalls = 0
            } catch (unnamed: UnsentReference) {
                Log.w(DRAIN_LOG_TAG, "A queued change names a row the server was never told about", unnamed)
                setAside(row, ApiErrorCodes.WRITE_UNSENDABLE, unnamed.messageOrType(), status = null)
                quarantined++
            } catch (unreachable: ApiException.Network) {
                return stall(row, unreachable, sent, quarantined)
            } catch (credentials: ApiException.Auth) {
                return needsCredentials(row, credentials, sent, quarantined)
            } catch (unconfigured: ApiException.SetupRequired) {
                return needsCredentials(row, unconfigured, sent, quarantined)
            } catch (throttled: ApiException.RateLimited) {
                return stall(row, throttled, sent, quarantined)
            } catch (broke: ApiException.Server) {
                val attempts = row.attempts + 1
                if (attempts < serverErrorLimit) return stall(row, broke, sent, quarantined, attempts)
                Log.w(DRAIN_LOG_TAG, "The server failed $attempts time(s) on the same change; setting it aside", broke)
                setAside(row.copy(attempts = attempts), broke.code, broke.messageOrType(), broke.status)
                quarantined++
            } catch (refused: ApiException) {
                Log.w(DRAIN_LOG_TAG, "The server refused a queued change: ${refused.code}", refused)
                setAside(row, quarantineCode(refused), refused.messageOrType(), refused.status, refused.blockerIds)
                quarantined++
            } catch (transport: IOException) {
                val unreachable =
                    ApiException.Network(transport.message ?: "the request did not reach the server", transport)
                return stall(row, unreachable, sent, quarantined)
            }
        }
    }

    /**
     * Sends one write and records what the server made of it.
     *
     * The ids of rows the write brought into existence are written onto the local
     * rows in the same step that takes the write out of the queue, so a process
     * that dies in between either has both or neither: a queue still holding the
     * write is a queue that will send it again under the same key, and the answer
     * that comes back names the same rows.
     */
    private suspend fun send(
        row: OutboxOpRow,
        op: OutboxOp,
    ) {
        val outcome = sender.send(op, row.id)
        val at = clock.now()
        db.withTransaction {
            for (assignment in outcome.assignments) ids.assign(assignment.ref, assignment.serverId, at)
            db.outbox().deleteById(row.id)
        }
        if (outcome.replayed) {
            Log.i(DRAIN_LOG_TAG, "The server had already applied ${row.op}; an earlier attempt did land")
        }
    }

    /** The op a stored payload denotes, or `null` when this build cannot read it. */
    private fun readable(row: OutboxOpRow): OutboxOp? =
        try {
            OutboxOpCodec.decode(row.payload)
        } catch (unreadable: IllegalArgumentException) {
            Log.w(DRAIN_LOG_TAG, "A queued change was written in a form this version cannot read", unreadable)
            null
        }

    private suspend fun markInFlight(row: OutboxOpRow) {
        db.outbox().update(row.copy(state = OutboxState.INFLIGHT, updatedAt = clock.now()))
    }

    /**
     * Leaves the write at the head of the queue and stops.
     *
     * [serverAttempts] is carried only when the server itself failed, because
     * that is the count that decides whether the write can ever land. A failure
     * to reach a server says nothing about the write and must not push it towards
     * being given up on — a week in a place with no signal is not a broken write.
     */
    private suspend fun stall(
        row: OutboxOpRow,
        cause: ApiException,
        sent: Int,
        quarantined: Int,
        serverAttempts: Int = row.attempts,
    ): DrainResult.Stalled {
        db.outbox().update(
            row.copy(
                state = OutboxState.FAILED,
                attempts = serverAttempts,
                lastError = cause.messageOrType(),
                updatedAt = clock.now(),
            ),
        )
        consecutiveStalls++
        val remaining = db.outbox().count()
        val retryAfter = backoff.delayAfter(consecutiveStalls)
        Log.i(
            DRAIN_LOG_TAG,
            "Held $remaining unsent change(s) after ${cause.code}; trying again in ${retryAfter}ms",
        )
        return DrainResult.Stalled(sent, quarantined, remaining, cause, retryAfter)
    }

    /** Puts the write back in line untouched: nothing about it is wrong, only who is asking. */
    private suspend fun needsCredentials(
        row: OutboxOpRow,
        cause: ApiException,
        sent: Int,
        quarantined: Int,
    ): DrainResult.NeedsCredentials {
        db.outbox().update(
            row.copy(state = OutboxState.PENDING, lastError = cause.messageOrType(), updatedAt = clock.now()),
        )
        val remaining = db.outbox().count()
        Log.w(DRAIN_LOG_TAG, "The server will not accept $remaining unsent change(s) from this session", cause)
        return DrainResult.NeedsCredentials(sent, quarantined, remaining, cause)
    }

    /**
     * Takes the write out of the queue and keeps it where the user can be shown it.
     *
     * [blockedBy] is kept beside the reason because a refusal on an open blocker
     * says the same sentence every time; which tasks were in the way is the only
     * part that tells the user what to do about it, and it is in the server's
     * answer to this one request and nowhere else afterwards.
     */
    private suspend fun setAside(
        row: OutboxOpRow,
        errorCode: String,
        errorMessage: String,
        status: Int?,
        blockedBy: List<Long> = emptyList(),
    ) {
        db.outbox().quarantine(row, errorCode, errorMessage, status, clock.now(), blockedBy)
    }

    /**
     * The reason a refused write is filed under.
     *
     * A write against a row the server no longer has is filed as the row being
     * gone rather than as a plain not-found, so it reads the same whether the
     * deletion was learned from a catch-up or from this refusal — the user is
     * told the thing they changed is no longer there, once, in one wording.
     */
    private fun quarantineCode(cause: ApiException): String =
        if (cause.status == HTTP_NOT_FOUND || cause.code == ApiErrorCodes.NOT_FOUND) {
            ApiErrorCodes.TARGET_GONE
        } else {
            cause.code
        }

    companion object {
        /**
         * How many times a server may fail on the same write before it is set
         * aside.
         *
         * A server error is worth retrying — the request may be fine and the
         * server merely unwell — but a write the server chokes on every time
         * would hold up everything queued behind it for as long as the app is
         * installed. Every attempt is a separate pass, minutes apart, so this is
         * a real chance to recover rather than three rapid retries.
         */
        const val DEFAULT_SERVER_ERROR_LIMIT: Int = 5

        private const val HTTP_NOT_FOUND: Int = 404

        private const val UNREADABLE_MESSAGE: String =
            "this version of the app cannot read the change that was saved"
    }
}

/** An exception's own words, or the name of its type when it had none. */
private fun Throwable.messageOrType(): String = message?.takeIf { it.isNotBlank() } ?: this::class.java.simpleName
