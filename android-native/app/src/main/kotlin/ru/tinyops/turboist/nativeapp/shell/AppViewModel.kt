package ru.tinyops.turboist.nativeapp.shell

import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dagger.hilt.android.lifecycle.HiltViewModel
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.nativeapp.auth.AuthStep
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.navigation.PendingTaskLinks
import ru.tinyops.turboist.nativeapp.session.OfflineSessionSource
import ru.tinyops.turboist.nativeapp.session.SessionState
import ru.tinyops.turboist.nativeapp.session.SessionStateSource
import ru.tinyops.turboist.nativeapp.settings.DeviceOptionsStore
import ru.tinyops.turboist.nativeapp.settings.SettingsRepository
import ru.tinyops.turboist.nativeapp.settings.ThemeChoice
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import ru.tinyops.turboist.nativeapp.sync.SyncStatus
import ru.tinyops.turboist.nativeapp.sync.SyncStatusSource
import javax.inject.Inject

/**
 * State for the top-level shell: which content tree to render, what the drawer
 * shows next to its entries, and the one question signing out has to ask.
 */
@HiltViewModel
class AppViewModel
    @Inject
    constructor(
        sessionStateSource: SessionStateSource,
        offlineSessionSource: OfflineSessionSource,
        drawerCountsSource: DrawerCountsSource,
        syncStatusSource: SyncStatusSource,
        deviceOptions: DeviceOptionsStore,
        settings: SettingsRepository,
        /**
         * A link from outside that had nowhere to land. It is handed to the app
         * shell rather than followed here: the shell owns the graph, and this
         * class owns no navigation of its own.
         */
        val pendingTaskLinks: PendingTaskLinks,
        private val session: SessionManager,
        private val sync: SyncScheduler,
    ) : ViewModel() {
        private val discardPrompt = MutableStateFlow<UnsentChangesPrompt?>(null)

        val sessionState: StateFlow<SessionState> = sessionStateSource.state

        /** How current the data on screen is, and how much of the user's work has not gone out. */
        val syncStatus: StateFlow<SyncStatus> = syncStatusSource.status

        /**
         * The drawer's counters, with the refused pile folded in.
         *
         * It arrives from the sync status rather than from the counts source
         * because it is not a count of anything in the workspace: it is a count
         * of what the device failed to tell the server, which is the one number
         * on the drawer that is about the app rather than about the work.
         */
        val drawerCounts: StateFlow<DrawerCounts> =
            combine(drawerCountsSource.counts, syncStatusSource.status) { counts, status ->
                counts.copy(unsentChanges = status.setAside)
            }.stateIn(viewModelScope, SharingStarted.WhileSubscribed(COUNTS_TIMEOUT_MILLIS), DrawerCounts())

        /**
         * Which colours the whole app is drawn in.
         *
         * It belongs here rather than to a screen because it wraps every tree the
         * shell can show, sign-in screens included, and because it is a choice
         * about this device: it must not wait for a session, a server or a sync.
         */
        val theme: StateFlow<ThemeChoice> =
            deviceOptions.observe()
                .map { it.theme }
                .stateIn(viewModelScope, SharingStarted.WhileSubscribed(COUNTS_TIMEOUT_MILLIS), ThemeChoice.SYSTEM)

        /**
         * Whether the daily plan is part of this installation's product.
         *
         * It is the user's own preference, read from the replicated document, so
         * a device that has not synced yet answers "off" and starts offering the
         * plan the moment the preference arrives. Hiding it until then is the
         * honest order of events: an entry leading to a plan the workspace may
         * not use is worse than one that appears a second late.
         */
        val dailyPlanEnabled: StateFlow<Boolean> =
            settings.observeUserSettings()
                .map { it.troikiEnabled }
                .stateIn(viewModelScope, SharingStarted.WhileSubscribed(COUNTS_TIMEOUT_MILLIS), false)

        /** True while the app is rendering a session it has not been able to check. */
        val unverifiedSession: StateFlow<Boolean> = offlineSessionSource.unverifiedSession

        /** Which sign-in screen the flow starts on when the shell shows it. */
        val authStep: StateFlow<AuthStep> = session.authStep

        /**
         * Runs a catch-up because the user asked for one from the status strip.
         *
         * The same cycle every automatic trigger runs — there is no such thing
         * as a different, harder try. What the button buys is not a different
         * request but the one the user is standing there waiting for, instead of
         * the one the schedule would have made later.
         */
        fun retrySync() {
            viewModelScope.launch { sync.requestSyncNow() }
        }

        /**
         * The confirmation signing out is waiting on, or `null` when it is not
         * waiting on one.
         *
         * Modelled as a value the shell renders rather than as a callback into a
         * dialog, so the sign-out that is in progress and the dialog on screen
         * cannot get out of step with each other.
         */
        val unsentChangesPrompt: StateFlow<UnsentChangesPrompt?> = discardPrompt.asStateFlow()

        /**
         * Signs out.
         *
         * This view model outlives the swap from the app to the sign-in screens —
         * it belongs to the activity, not to a destination — so the wipe cannot be
         * cancelled halfway by the very state change it causes, which would leave a
         * device with no token and a full replica.
         */
        fun signOut() {
            viewModelScope.launch {
                session.logOut { unsent ->
                    val answer = CompletableDeferred<Boolean>()
                    discardPrompt.value = UnsentChangesPrompt(unsent, answer)
                    try {
                        answer.await()
                    } finally {
                        discardPrompt.value = null
                    }
                }
            }
        }
    }

/**
 * A sign-out paused on the user's answer.
 *
 * @property count how many local changes the server has not accepted. They are
 *   lost by the wipe, so the number is shown rather than a vague warning.
 */
class UnsentChangesPrompt(
    val count: Int,
    private val answer: CompletableDeferred<Boolean>,
) {
    fun discard() {
        answer.complete(true)
    }

    fun keep() {
        answer.complete(false)
    }
}

/** Long enough to survive a screen rotation without tearing the counters down. */
private const val COUNTS_TIMEOUT_MILLIS = 5_000L
