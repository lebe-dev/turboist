package ru.tinyops.turboist.nativeapp.sync

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.distinctUntilChanged
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.update
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncCycleResult
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Whether a cycle is running, and how the last one ended.
 *
 * The engine answers whoever asked for a cycle and keeps no history, which is
 * right for an engine and useless for a strip at the top of the screen: the
 * screen was not the one that asked, and the cycles it needs to report on are
 * the automatic ones nobody asked for. So the cycle is wrapped once, at the
 * point where the app assembles it, and what it reports is published here.
 *
 * One instance for the process. Two would each see half the cycles and neither
 * would be able to say whether the app is up to date.
 */
@Singleton
class SyncActivity
    @Inject
    constructor() {
        /**
         * How many cycles are on the wire. A count rather than a flag because a
         * manual refresh and a scheduled run can overlap, and a flag cleared by
         * whichever finished first would leave the spinner off while the other
         * one was still going.
         */
        private val running = MutableStateFlow(0)

        private val settled = MutableStateFlow(SyncOutcome.UNKNOWN)

        /** True for as long as at least one cycle is running. */
        val syncing: Flow<Boolean> = running.map { it > 0 }.distinctUntilChanged()

        /** How the last cycle to finish ended. */
        val outcome: StateFlow<SyncOutcome> = settled.asStateFlow()

        /**
         * The same cycle, reporting what it is doing.
         *
         * A decorator rather than a hook inside the engine: what a cycle is stays
         * one thing, and an app that did not want the reporting would simply not
         * wrap it.
         *
         * A cycle that throws is reported as a turn the server did not complete
         * and the failure is passed on unchanged, because the caller above still
         * has to decide what to do with it — swallowing it here would make a
         * defect look like an ordinary refusal at every level above.
         */
        fun watching(cycle: SyncCycle): SyncCycle =
            SyncCycle {
                running.update { it + 1 }
                try {
                    cycle.runSyncCycle().also { settled.value = it.asOutcome() }
                } catch (cancelled: CancellationException) {
                    // The scope is shutting down. Nothing concluded, so nothing
                    // is published: the last real answer stays the last answer.
                    throw cancelled
                } catch (fault: Exception) {
                    settled.value = SyncOutcome.REFUSED
                    throw fault
                } finally {
                    running.update { it - 1 }
                }
            }
    }

/** How a cycle's own answer reads as something a screen can show. */
private fun SyncCycleResult.asOutcome(): SyncOutcome =
    when (this) {
        SyncCycleResult.Synced -> SyncOutcome.SYNCED
        SyncCycleResult.Offline -> SyncOutcome.UNREACHABLE
        is SyncCycleResult.Refused -> SyncOutcome.REFUSED
    }
