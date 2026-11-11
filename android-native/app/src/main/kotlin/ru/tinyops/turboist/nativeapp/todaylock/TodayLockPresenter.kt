package ru.tinyops.turboist.nativeapp.todaylock

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.TodayLock
import ru.tinyops.turboist.core.model.diagnostics.Faults
import java.time.Instant

/** What the Today lock control says back after the user acted on it. */
enum class TodayLockMessage {
    /** The lock is on until midnight. */
    LOCKED,

    /** The lock could not be turned on. */
    FAILED,
}

/**
 * The Today focus lock, as the control on the Today screen reads and changes it.
 *
 * Whether a lock runs is a fact about two things that change on their own: the
 * replicated preference saying when it ends, and the clock. Both are followed,
 * so the control turns itself off at midnight on a screen left open overnight.
 *
 * Turning it on is a local write like any other — the end is written into the
 * replica and the request queued — so it holds from the tap, with or without a
 * network. There is no way to turn it off: it runs out on its own.
 */
class TodayLockPresenter(
    private val scope: CoroutineScope,
    lockUntil: Flow<Long?>,
    now: Flow<Instant>,
    private val lockToday: suspend () -> Unit,
) {
    /** Whether a lock runs right now. */
    val active: StateFlow<Boolean> =
        combine(lockUntil, now) { until, at -> TodayLock.isActive(until, at.toEpochMilli()) }
            .distinctUntilChanged()
            .stateIn(scope, SharingStarted.Eagerly, false)

    private val outgoing = MutableSharedFlow<TodayLockMessage>(extraBufferCapacity = 1)

    /** One message per act of the user, to be shown once. */
    val messages: SharedFlow<TodayLockMessage> = outgoing.asSharedFlow()

    /** Turns the lock on. A running lock is left as it is and nothing is said. */
    @Suppress("TooGenericExceptionCaught")
    fun lock() {
        if (active.value) return
        scope.launch {
            try {
                lockToday()
                outgoing.emit(TodayLockMessage.LOCKED)
            } catch (failure: RuntimeException) {
                Faults.report("todaylock", "Turning the Today lock on failed", failure)
                outgoing.emit(TodayLockMessage.FAILED)
            }
        }
    }
}
