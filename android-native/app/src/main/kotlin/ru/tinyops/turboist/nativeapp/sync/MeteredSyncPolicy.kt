package ru.tinyops.turboist.nativeapp.sync

import ru.tinyops.turboist.core.sync.trigger.BackgroundSyncSchedule
import ru.tinyops.turboist.nativeapp.settings.DeviceOptionsStore
import java.util.concurrent.atomic.AtomicBoolean
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Whether the jobs the system runs on the app's behalf may spend mobile data.
 *
 * The answer has to be readable without suspending, because it is read while a
 * job is being handed to the platform's scheduler, so it is held here as a plain
 * flag and kept in step with the stored choice by the two calls below.
 *
 * It governs the *background* only. A catch-up the user asked for by pulling a
 * list down is a person standing there waiting, and refusing that because the
 * phone is on mobile data would be an app that stopped working rather than one
 * that stopped costing.
 */
@Singleton
class MeteredSyncPolicy
    @Inject
    constructor(
        private val options: DeviceOptionsStore,
        private val schedule: BackgroundSyncSchedule,
    ) {
        private val allowed = AtomicBoolean(true)

        /** What the scheduler asks each time it builds a job. Never suspends. */
        fun meteredAllowed(): Boolean = allowed.get()

        /**
         * Loads the stored choice.
         *
         * Awaited before the triggers are armed, so the first repeating job the
         * process enqueues already carries the constraint the user chose. A job
         * enqueued first and corrected afterwards would run once under the wrong
         * rule, which for this setting means exactly the data spend it exists to
         * prevent.
         */
        suspend fun load() {
            allowed.set(options.read().syncOnMetered)
        }

        /**
         * Applies a change the user just made.
         *
         * The repeating job is torn down and asked for again rather than left
         * alone: its constraints are fixed at the moment it is enqueued, and the
         * schedule keeps an existing job instead of replacing it — so without
         * this the new answer would only take effect the next time the user
         * signed in.
         */
        fun apply(meteredAllowed: Boolean) {
            allowed.set(meteredAllowed)
            schedule.stopSyncing()
            schedule.keepSyncing()
        }
    }
