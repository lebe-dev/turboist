package ru.tinyops.turboist.nativeapp.di

import android.content.Context
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.ProcessLifecycleOwner
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.android.qualifiers.ApplicationContext
import dagger.hilt.components.SingletonComponent
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.filter
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.TurboistDatabase
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.network.api.SyncApi
import ru.tinyops.turboist.core.network.events.ChangeStream
import ru.tinyops.turboist.core.network.events.EventStream
import ru.tinyops.turboist.core.sync.SyncCycle
import ru.tinyops.turboist.core.sync.SyncMutex
import ru.tinyops.turboist.core.sync.drain.OpSender
import ru.tinyops.turboist.core.sync.drain.OutboxDrainer
import ru.tinyops.turboist.core.sync.drain.UnsentChanges
import ru.tinyops.turboist.core.sync.maintenance.CompletedHistoryPrune
import ru.tinyops.turboist.core.sync.maintenance.withMaintenance
import ru.tinyops.turboist.core.sync.pull.ReplicaApplier
import ru.tinyops.turboist.core.sync.pull.SyncPuller
import ru.tinyops.turboist.core.sync.trigger.BackgroundSyncSchedule
import ru.tinyops.turboist.core.sync.trigger.ConnectivityWatcher
import ru.tinyops.turboist.core.sync.trigger.NetworkAvailability
import ru.tinyops.turboist.core.sync.trigger.SyncScheduler
import ru.tinyops.turboist.core.sync.trigger.SyncTriggers
import ru.tinyops.turboist.core.sync.trigger.WorkManagerSyncSchedule
import ru.tinyops.turboist.core.sync.write.ContextWriteRepo
import ru.tinyops.turboist.core.sync.write.LabelWriteRepo
import ru.tinyops.turboist.core.sync.write.OutboxWriter
import ru.tinyops.turboist.core.sync.write.ProjectWriteRepo
import ru.tinyops.turboist.core.sync.write.RecurrenceAdvancer
import ru.tinyops.turboist.core.sync.write.ReplicaServerIds
import ru.tinyops.turboist.core.sync.write.SettingsWriteRepo
import ru.tinyops.turboist.core.sync.write.TaskWriteRepo
import ru.tinyops.turboist.core.sync.write.TemplateDrafts
import ru.tinyops.turboist.core.sync.write.TemplateWriteRepo
import ru.tinyops.turboist.core.sync.write.TroikiWriteRepo
import ru.tinyops.turboist.nativeapp.session.SessionState
import ru.tinyops.turboist.nativeapp.session.SessionStateSource
import ru.tinyops.turboist.nativeapp.sync.MeteredSyncPolicy
import ru.tinyops.turboist.nativeapp.sync.SyncActivity
import java.time.Clock
import javax.inject.Provider
import javax.inject.Qualifier
import javax.inject.Singleton

/** Whether the app is in front of the user, as a stream of true and false. */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class ForegroundState

/** Whether there is a session to sync with. */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class SignedInState

/**
 * The sync engine, assembled.
 *
 * Two things here are singletons for a reason rather than by habit. The lock is
 * the whole of the engine's mutual exclusion, so a second instance would be the
 * same as none. The scheduler owns the window that folds a burst of reasons into
 * one cycle, and a second one would open a window of its own and undo exactly
 * the coalescing it exists for.
 */
@Module
@InstallIn(SingletonComponent::class)
object SyncModule {
    @Provides
    @Singleton
    fun syncMutex(): SyncMutex = SyncMutex()

    @Provides
    @Singleton
    fun replicaApplier(database: TurboistDatabase): ReplicaApplier = ReplicaApplier(database)

    /**
     * The one queue every unsent write goes through. A second instance would
     * hand out its own op ids and open its own transactions, so the optimistic
     * change and the op recording it could stop committing together.
     */
    @Provides
    @Singleton
    fun outboxWriter(database: TurboistDatabase): OutboxWriter = OutboxWriter(database)

    /**
     * The write path for tasks, given the ability to work out when a repeating
     * task next falls due.
     *
     * Without it a task ticked off with no connection would sit unchanged until
     * the queue drained. The device's own zone is used for the calculation and
     * for counting a day, and the two are deliberately the same one: a rule
     * repeats a wall clock, and the wall clock the user reads is the device's.
     */
    @Provides
    @Singleton
    fun taskWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
        clock: Clock,
    ): TaskWriteRepo =
        TaskWriteRepo(
            db = database,
            writer = writer,
            recurrence = RecurrenceAdvancer(clock.zone),
            zone = clock.zone,
        )

    /**
     * The write path for projects, their boards and the contexts above them.
     * One instance for the process, for the same reason the queue is one: they
     * share it, and a second would open transactions of its own.
     */
    @Provides
    @Singleton
    fun projectWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): ProjectWriteRepo = ProjectWriteRepo(db = database, writer = writer)

    @Provides
    @Singleton
    fun contextWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): ContextWriteRepo = ContextWriteRepo(db = database, writer = writer)

    /**
     * The write path for labels themselves — naming one, recolouring it, taking
     * it away. Attaching a label to a task is a change to the task and goes
     * through the task write path instead.
     */
    @Provides
    @Singleton
    fun labelWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): LabelWriteRepo = LabelWriteRepo(db = database, writer = writer)

    /**
     * The write path for the reusable blueprints, and the reader that cuts a
     * draft of one out of work that already exists. The reader is separate
     * because it writes nothing: it only reads the replica, which is what lets a
     * template be cut from a task the server has never heard of.
     */
    @Provides
    @Singleton
    fun templateWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): TemplateWriteRepo = TemplateWriteRepo(db = database, writer = writer)

    @Provides
    @Singleton
    fun templateDrafts(database: TurboistDatabase): TemplateDrafts = TemplateDrafts(database)

    /** The write path for the daily plan's own two controls: begin a cycle, end one. */
    @Provides
    @Singleton
    fun troikiWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): TroikiWriteRepo = TroikiWriteRepo(db = database, writer = writer)

    /**
     * The write path for the preference documents and for the jump pair. One
     * instance for the process, for the same reason the queue is one.
     */
    @Provides
    @Singleton
    fun settingsWriteRepo(
        database: TurboistDatabase,
        writer: OutboxWriter,
    ): SettingsWriteRepo = SettingsWriteRepo(db = database, writer = writer)

    @Provides
    @Singleton
    fun syncPuller(
        api: SyncApi,
        database: TurboistDatabase,
        applier: ReplicaApplier,
        mutex: SyncMutex,
    ): SyncPuller = SyncPuller(api = api, db = database, applier = applier, mutex = mutex)

    /** The device's own ids translated into the server's, at the moment of sending. */
    @Provides
    @Singleton
    fun replicaServerIds(database: TurboistDatabase): ReplicaServerIds = ReplicaServerIds(database)

    @Provides
    @Singleton
    fun opSender(
        network: TurboistNetwork,
        ids: ReplicaServerIds,
    ): OpSender = OpSender(network = network, ids = ids)

    /**
     * One drainer for the process. It remembers how long the queue has been
     * waiting, and a second instance would start that count again on every
     * attempt and so never back off at all.
     */
    @Provides
    @Singleton
    fun outboxDrainer(
        database: TurboistDatabase,
        sender: OpSender,
        ids: ReplicaServerIds,
        mutex: SyncMutex,
    ): OutboxDrainer = OutboxDrainer(db = database, sender = sender, ids = ids, mutex = mutex)

    @Provides
    @Singleton
    fun unsentChanges(database: TurboistDatabase): UnsentChanges = UnsentChanges(database)

    /**
     * Keeping the device's copy of finished work the same size as the server's.
     *
     * The device's own clock is what measures the window, and the device's own
     * zone is what a day is counted in — the same pair the history screen reads
     * it with, so the two cannot disagree about where the window begins.
     */
    @Provides
    @Singleton
    fun completedHistoryPrune(
        database: TurboistDatabase,
        clock: Clock,
    ): CompletedHistoryPrune = CompletedHistoryPrune(db = database, clock = clock)

    /**
     * One turn of the engine: send what is queued, read what changed, then tidy
     * up. Housekeeping rides on the cycle rather than on a schedule of its own
     * because it is only worth doing, and only safe to do, once the two sides are
     * known to agree.
     *
     * The whole thing is wrapped once so that what it is doing can be observed.
     * Wrapping here rather than inside the engine keeps what a cycle *is* in one
     * piece, and puts the reporting where the app is assembled — which is the
     * only place that knows a screen is going to be watching.
     */
    @Provides
    @Singleton
    fun syncCycle(
        drainer: OutboxDrainer,
        puller: SyncPuller,
        prune: CompletedHistoryPrune,
        activity: SyncActivity,
    ): SyncCycle =
        activity.watching(
            SyncCycle.sending(drainer = drainer, puller = puller).withMaintenance(prune),
        )

    @Provides
    @Singleton
    fun syncScheduler(
        cycle: SyncCycle,
        @ApplicationScope scope: CoroutineScope,
    ): SyncScheduler = SyncScheduler(cycle = cycle, scope = scope)

    @Provides
    @Singleton
    fun changeStream(
        network: TurboistNetwork,
        serverUrl: ServerUrl,
    ): ChangeStream = EventStream(api = network.events, serverUrl = serverUrl, httpClient = network.httpClient)

    @Provides
    @Singleton
    fun networkAvailability(
        @ApplicationContext context: Context,
    ): NetworkAvailability = ConnectivityWatcher(context)

    @Provides
    @Singleton
    fun backgroundSyncSchedule(
        @ApplicationContext context: Context,
        // Asked for indirectly because the policy needs the schedule in order to
        // re-apply a changed answer to the job already enqueued. The lambda is
        // only ever run while a job is being built, long after both exist.
        metered: Provider<MeteredSyncPolicy>,
    ): BackgroundSyncSchedule = WorkManagerSyncSchedule.of(context) { metered.get().meteredAllowed() }

    /**
     * The app is in front of the user whenever the process's lifecycle is
     * started. Process-wide rather than per-activity: rotating the screen
     * destroys and recreates an activity, and a change stream torn down and
     * handshaked again for that would be pure waste.
     */
    @Provides
    @Singleton
    @ForegroundState
    fun foregroundState(): Flow<Boolean> =
        ProcessLifecycleOwner.get().lifecycle.currentStateFlow.map { it.isAtLeast(Lifecycle.State.STARTED) }

    /**
     * Whether there is a session, asked only once someone has looked.
     *
     * The unresolved state is dropped rather than reported as "no session".
     * They call for opposite actions — no session stops every background job,
     * unresolved must leave them alone — and a process the system started to run
     * a sync job begins unresolved, because the stored credentials are read from
     * disk. Reporting a guess there makes such a process cancel, as its first
     * act, the very job that started it: the catch-up is thrown away and the
     * schedule is rebuilt from scratch, so a device woken from a dead process
     * loses the sync it was woken for.
     */
    @Provides
    @Singleton
    @SignedInState
    fun signedInState(sessions: SessionStateSource): Flow<Boolean> =
        sessions.state
            .filter { it != SessionState.Connecting }
            .map { it == SessionState.LoggedIn }

    @Provides
    @Singleton
    fun syncTriggers(
        stream: ChangeStream,
        network: NetworkAvailability,
        schedule: BackgroundSyncSchedule,
        scheduler: SyncScheduler,
        @SignedInState signedIn: Flow<Boolean>,
        @ForegroundState foreground: Flow<Boolean>,
    ): SyncTriggers =
        SyncTriggers(
            stream = stream,
            network = network,
            schedule = schedule,
            scheduler = scheduler,
            signedIn = signedIn,
            foreground = foreground,
        )
}
