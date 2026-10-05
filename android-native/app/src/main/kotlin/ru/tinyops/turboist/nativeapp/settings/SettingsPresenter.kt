package ru.tinyops.turboist.nativeapp.settings

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharedFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asSharedFlow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.DayPart
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.MAX_MAX_PINNED
import ru.tinyops.turboist.core.model.MIN_MAX_PINNED
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import ru.tinyops.turboist.core.sync.write.WriteRefused

/** Something the settings screen has to say back after an action. */
sealed interface SettingsMessage {
    data object Saved : SettingsMessage

    data object SaveFailed : SettingsMessage

    /** A pinning cap outside the range the server accepts, so nothing was written. */
    data object PinnedCapOutOfRange : SettingsMessage

    /** A rule with no mask or nothing to apply, which the server would refuse. */
    data object RuleIncomplete : SettingsMessage

    /** The on-device copy was emptied and asked for again. */
    data object LocalDataCleared : SettingsMessage
}

/** Which of the two rule lists an editor is open on. */
enum class RuleKind {
    /**
     * Applied on create. A task whose title matches comes out already carrying
     * the labels, without anyone being asked.
     */
    AUTO_LABEL,

    /**
     * Never applied. A match only offers the projects while the user is typing,
     * and a project reaches the task because the user picked it.
     */
    PROJECT_SUGGESTION,
}

/**
 * A rule being written.
 *
 * @property index the position of the rule being changed, or `null` while a new
 *   one is being written — the list is replaced whole on save, so a rule's
 *   position in it is its identity.
 * @property targetServerIds the labels or projects the rule names, as **server**
 *   ids: the rules document belongs to the server and travels back to it
 *   unchanged, so a local id in it would be meaningless there.
 */
data class RuleEditor(
    val kind: RuleKind,
    val index: Int? = null,
    val mask: String = "",
    val targetServerIds: List<Long> = emptyList(),
    val ignoreCase: Boolean = false,
) {
    /** The server refuses a rule with no mask or nothing to apply, so this screen does too. */
    val canSave: Boolean get() = mask.isNotBlank() && targetServerIds.isNotEmpty()
}

/** A destructive action waiting to be confirmed. */
enum class SettingsConfirmation {
    /** Points the app at a different installation, which empties this one's copy. */
    CHANGE_SERVER,

    /** Throws the on-device copy away and asks the server for it again. */
    CLEAR_LOCAL_DATA,
}

/** Everything the settings screen renders. */
data class SettingsUiState(
    val loading: Boolean = true,
    /** The user's own preferences. Never merged with [app]: different document, different owner. */
    val user: UserSettings = UserSettings(),
    /** The installation's rules, which every user of this server obeys. */
    val app: AppSettings = AppSettings(),
    /** The choices that belong to this phone and never leave it. */
    val device: DeviceOptions = DeviceOptions(),
    val knownLabels: List<Label> = emptyList(),
    val knownProjects: List<Project> = emptyList(),
    val editor: RuleEditor? = null,
    val confirming: SettingsConfirmation? = null,
    /** How many changes the server has not accepted, shown before anything that would discard them. */
    val unsentChangeCount: Int = 0,
    val serverAddress: String = "",
    val version: String = "",
    /** What is being typed into the banner field, or `null` while nothing is. */
    val bannerDraft: String? = null,
    val pinnedTasksDraft: String? = null,
    val pinnedProjectsDraft: String? = null,
) {
    /** What the banner field shows: what is being typed, or the stored text when nothing is. */
    val bannerText: String get() = bannerDraft ?: user.bannerText

    val maxPinnedTasks: String get() = pinnedTasksDraft ?: user.maxPinnedTasks.toString()

    val maxPinnedProjects: String get() = pinnedProjectsDraft ?: user.maxPinnedProjects.toString()

    /** True while the banner field holds something other than what is stored. */
    val bannerEdited: Boolean get() = bannerDraft != null && bannerDraft != user.bannerText

    /** True while either cap field holds something other than what is stored. */
    val pinnedCapsEdited: Boolean
        get() =
            (pinnedTasksDraft != null && pinnedTasksDraft != user.maxPinnedTasks.toString()) ||
                (pinnedProjectsDraft != null && pinnedProjectsDraft != user.maxPinnedProjects.toString())

    /**
     * The labels a preference or a rule may name.
     *
     * A label this device created while offline is not offered: it has no server
     * id yet, and the documents these lists go into are addressed by server ids
     * only. It appears the moment the queue drains.
     */
    val nameableLabels: List<Label> get() = knownLabels.filter { it.serverId != null }

    /** The projects a suggestion rule may offer, for the same reason. */
    val nameableProjects: List<Project> get() = knownProjects.filter { it.serverId != null }

    val termsUrl: String get() = LegalDocuments.urlFor(serverAddress, LegalDocuments.TERMS_PATH)

    val privacyUrl: String get() = LegalDocuments.urlFor(serverAddress, LegalDocuments.PRIVACY_PATH)
}

/**
 * The behaviour of the settings screen.
 *
 * A plain object driven by a scope, like every other presenter here, so all of
 * it runs without Compose and without the platform.
 *
 * One screen sits over three stores that must never be confused for each other:
 *
 * - the **user's own preferences**, a document on the server that follows the
 *   account between devices;
 * - the **installation's rules**, a second document that belongs to the server
 *   rather than to a person, replaced whole through endpoints of its own;
 * - the **device's own choices**, which are never sent anywhere.
 *
 * Each has its own write path here and they are never combined into one save.
 * Merging them would be the bug the separation exists to prevent: a screen that
 * wrote all three at once would sooner or later hand one document the other's
 * contents.
 *
 * Every server-side edit is a change to one key. The request carries only what
 * changed, so a preference this build has never heard of survives an edit made
 * from it — which matters here more than anywhere, because the phone and the
 * server it talks to are upgraded on different days.
 */
class SettingsPresenter(
    private val scope: CoroutineScope,
    userSettings: Flow<UserSettings>,
    appSettings: Flow<AppSettings>,
    labels: Flow<List<Label>>,
    projects: Flow<List<Project>>,
    deviceOptions: Flow<DeviceOptions>,
    private val actions: SettingsActions,
    private val device: DeviceOptionActions,
    private val connection: ServerConnection,
    private val release: AppRelease,
) {
    /** The part of the screen's state that is being typed rather than stored. */
    private data class Local(
        val bannerDraft: String? = null,
        val pinnedTasksDraft: String? = null,
        val pinnedProjectsDraft: String? = null,
        val editor: RuleEditor? = null,
        val confirming: SettingsConfirmation? = null,
        val unsentChangeCount: Int = 0,
        val serverAddress: String = "",
        val version: String = "",
    )

    private val local = MutableStateFlow(Local())
    private val outgoing = MutableSharedFlow<SettingsMessage>(extraBufferCapacity = 2)

    private val stored: Flow<SettingsUiState> =
        combine(userSettings, appSettings, labels, projects, deviceOptions) { user, app, known, places, options ->
            SettingsUiState(
                loading = false,
                user = user,
                app = app,
                device = options,
                knownLabels = known,
                knownProjects = places,
            )
        }

    /** What the screen renders. */
    val state: StateFlow<SettingsUiState> =
        combine(stored, local) { rendered, own ->
            rendered.copy(
                editor = own.editor,
                confirming = own.confirming,
                unsentChangeCount = own.unsentChangeCount,
                serverAddress = own.serverAddress,
                version = own.version,
                bannerDraft = own.bannerDraft,
                pinnedTasksDraft = own.pinnedTasksDraft,
                pinnedProjectsDraft = own.pinnedProjectsDraft,
            )
        }.stateIn(scope, SharingStarted.WhileSubscribed(STOP_TIMEOUT_MILLIS), SettingsUiState())

    /** What the screen says back, once each. */
    val messages: SharedFlow<SettingsMessage> = outgoing.asSharedFlow()

    init {
        scope.launch {
            local.value =
                local.value.copy(serverAddress = connection.address(), version = release.versionName())
        }
    }

    // --- the user's own preferences -----------------------------------------

    fun setLocale(locale: String) = patch(PatchUserSettingsRequest(locale = locale))

    fun setPublicView(on: Boolean) = patch(PatchUserSettingsRequest(publicView = on))

    fun setTroikiEnabled(on: Boolean) = patch(PatchUserSettingsRequest(troikiEnabled = on))

    fun setCalendarEnabled(on: Boolean) = patch(PatchUserSettingsRequest(calendarEnabled = on))

    fun setCalendarHidePastEvents(on: Boolean) = patch(PatchUserSettingsRequest(calendarHidePastEvents = on))

    fun setBannerPublished(on: Boolean) = patch(PatchUserSettingsRequest(bannerPublished = on))

    /**
     * Narrows the Today banner to one phase of the day, or lets it run all day.
     *
     * All day is the empty string on the wire, which is a different thing from
     * the `none` phase — that is why the absent value is spelled as `null` here
     * and never as [DayPart.NONE].
     */
    fun setBannerDayPart(dayPart: DayPart?) = patch(PatchUserSettingsRequest(bannerDayPart = dayPart?.wire.orEmpty()))

    fun editBannerText(text: String) {
        local.value = local.value.copy(bannerDraft = text)
    }

    /** Saves the banner text that has been typed. Nothing is written while it matches what is stored. */
    fun saveBannerText() {
        val typed = local.value.bannerDraft ?: return
        patch(PatchUserSettingsRequest(bannerText = typed)) { local.value = local.value.copy(bannerDraft = null) }
    }

    /**
     * Puts a label on one of the preference lists, or takes it off again.
     *
     * The whole list is sent because that is what the key holds; only that one
     * key is sent, because that is all that changed.
     */
    fun toggleWeeklyExcludedLabel(labelServerId: Long) {
        val current = state.value.user.weeklyUnplannedExcludedLabelIds
        patch(PatchUserSettingsRequest(weeklyUnplannedExcludedLabelIds = toggled(current, labelServerId)))
    }

    fun toggleBugLabel(labelServerId: Long) {
        val current = state.value.user.bugLabelIds
        patch(PatchUserSettingsRequest(bugLabelIds = toggled(current, labelServerId)))
    }

    fun editMaxPinnedTasks(text: String) {
        local.value = local.value.copy(pinnedTasksDraft = text)
    }

    fun editMaxPinnedProjects(text: String) {
        local.value = local.value.copy(pinnedProjectsDraft = text)
    }

    /**
     * Saves both pinning caps.
     *
     * A value outside the range the server accepts is refused here and nothing is
     * written. That is deliberately not the same as what happens to a *stored*
     * value out of range, which is read back as the default: a blob written
     * before the setting existed is a gap to fill in, while a number a person
     * just typed is a mistake to point at rather than silently replace.
     */
    fun savePinnedCaps() {
        // Read from what is being typed rather than from the rendered state: the
        // rendered copy is produced asynchronously, so a save made in the same
        // breath as the last keystroke would otherwise send the value before it.
        val stored = state.value.user
        val tasks = capOf(local.value.pinnedTasksDraft ?: stored.maxPinnedTasks.toString())
        val projects = capOf(local.value.pinnedProjectsDraft ?: stored.maxPinnedProjects.toString())
        if (tasks == null || projects == null) {
            scope.launch { outgoing.emit(SettingsMessage.PinnedCapOutOfRange) }
            return
        }
        patch(PatchUserSettingsRequest(maxPinnedTasks = tasks, maxPinnedProjects = projects)) {
            local.value = local.value.copy(pinnedTasksDraft = null, pinnedProjectsDraft = null)
        }
    }

    // --- the installation's rules -------------------------------------------

    /** Opens the editor on a new rule of one kind. */
    fun startNewRule(kind: RuleKind) {
        local.value = local.value.copy(editor = RuleEditor(kind = kind))
    }

    /** Opens the editor on a rule that exists, filled in with what it holds. */
    fun editRule(
        kind: RuleKind,
        index: Int,
    ) {
        val editor =
            when (kind) {
                RuleKind.AUTO_LABEL ->
                    state.value.app.autoLabels.getOrNull(index)?.let {
                        RuleEditor(kind, index, it.mask, it.labelIds, it.ignoreCase)
                    }

                RuleKind.PROJECT_SUGGESTION ->
                    state.value.app.projectSuggestions.getOrNull(index)?.let {
                        RuleEditor(kind, index, it.mask, it.projectIds, it.ignoreCase)
                    }
            } ?: return
        local.value = local.value.copy(editor = editor)
    }

    fun cancelRule() {
        local.value = local.value.copy(editor = null)
    }

    fun setRuleMask(mask: String) = editEditor { it.copy(mask = mask) }

    fun setRuleIgnoreCase(ignoreCase: Boolean) = editEditor { it.copy(ignoreCase = ignoreCase) }

    /** Names one more label or project in the rule being written, or stops naming it. */
    fun toggleRuleTarget(serverId: Long) =
        editEditor { it.copy(targetServerIds = toggled(it.targetServerIds, serverId)) }

    /**
     * Saves the rule being written into its list.
     *
     * The whole list goes to the server, because the endpoint takes the whole
     * list; only the list the editor belongs to is touched, because the two lists
     * have endpoints of their own and writing both would replace one of them with
     * whatever this screen last read.
     */
    fun saveRule() {
        val editor = local.value.editor ?: return
        if (!editor.canSave) {
            scope.launch { outgoing.emit(SettingsMessage.RuleIncomplete) }
            return
        }
        val mask = editor.mask.trim()
        scope.launch {
            val written =
                report {
                    when (editor.kind) {
                        RuleKind.AUTO_LABEL -> {
                            val rule = AutoLabelRule(mask, editor.targetServerIds, editor.ignoreCase)
                            actions.putAutoLabels(replaced(state.value.app.autoLabels, editor.index, rule))
                        }

                        RuleKind.PROJECT_SUGGESTION -> {
                            val rule = ProjectSuggestionRule(mask, editor.targetServerIds, editor.ignoreCase)
                            actions.putProjectSuggestions(
                                replaced(state.value.app.projectSuggestions, editor.index, rule),
                            )
                        }
                    }
                }
            if (!written) return@launch
            local.value = local.value.copy(editor = null)
            outgoing.emit(SettingsMessage.Saved)
        }
    }

    fun deleteRule(
        kind: RuleKind,
        index: Int,
    ) {
        scope.launch {
            val written =
                report {
                    when (kind) {
                        RuleKind.AUTO_LABEL ->
                            actions.putAutoLabels(state.value.app.autoLabels.filterIndexed { at, _ -> at != index })

                        RuleKind.PROJECT_SUGGESTION ->
                            actions.putProjectSuggestions(
                                state.value.app.projectSuggestions.filterIndexed { at, _ -> at != index },
                            )
                    }
                }
            if (written) outgoing.emit(SettingsMessage.Saved)
        }
    }

    // --- this device --------------------------------------------------------

    fun setTheme(theme: ThemeChoice) {
        scope.launch { device.setTheme(theme) }
    }

    fun setSyncOnMetered(allowed: Boolean) {
        scope.launch { device.setSyncOnMetered(allowed) }
    }

    /**
     * Asks before anything that empties the device.
     *
     * The count of what has not reached the server is read at this moment rather
     * than kept up to date: it is only ever shown inside this question, and a
     * number read now is the one the user is actually agreeing to.
     */
    fun ask(confirmation: SettingsConfirmation) {
        scope.launch {
            val unsent = runCatching { connection.unsentChangeCount() }.getOrDefault(0)
            local.value = local.value.copy(confirming = confirmation, unsentChangeCount = unsent)
        }
    }

    fun cancelConfirmation() {
        local.value = local.value.copy(confirming = null)
    }

    /** Runs the destructive action the user was asked about. */
    fun confirm() {
        val pending = local.value.confirming ?: return
        scope.launch {
            local.value = local.value.copy(confirming = null)
            when (pending) {
                // Nothing is emitted afterwards: the app has no server, so the
                // shell has already swapped this screen for the one that asks for
                // an address.
                SettingsConfirmation.CHANGE_SERVER -> report { connection.forgetServer() }

                SettingsConfirmation.CLEAR_LOCAL_DATA ->
                    if (report { connection.clearLocalData() }) {
                        outgoing.emit(SettingsMessage.LocalDataCleared)
                    }
            }
        }
    }

    // --- internals -----------------------------------------------------------

    /**
     * Sends one change to the user's own preferences.
     *
     * [onWritten] runs only when the write happened, which is what lets a field
     * being typed into let go of its draft without ever losing what the user
     * wrote to a refusal.
     */
    private fun patch(
        edit: PatchUserSettingsRequest,
        onWritten: () -> Unit = {},
    ) {
        scope.launch {
            if (!report { actions.patchUserSettings(edit) }) return@launch
            onWritten()
            outgoing.emit(SettingsMessage.Saved)
        }
    }

    private fun editEditor(change: (RuleEditor) -> RuleEditor) {
        local.value = local.value.copy(editor = local.value.editor?.let(change))
    }

    /**
     * Runs a write and says so when it did not happen.
     *
     * Nothing a settings write can be refused for is something the user can act
     * on — a preference names no other row and takes up no capacity — so there is
     * one sentence for all of them rather than a false distinction.
     */
    private suspend fun report(write: suspend () -> Unit): Boolean {
        try {
            write()
            return true
        } catch (refusal: WriteRefused) {
            outgoing.emit(SettingsMessage.SaveFailed)
        } catch (failure: RuntimeException) {
            outgoing.emit(SettingsMessage.SaveFailed)
        }
        return false
    }

    private fun toggled(
        ids: List<Long>,
        id: Long,
    ): List<Long> = if (id in ids) ids - id else ids + id

    private fun <T> replaced(
        rules: List<T>,
        index: Int?,
        rule: T,
    ): List<T> =
        if (index == null || index !in rules.indices) {
            rules + rule
        } else {
            rules.mapIndexed { at, existing -> if (at == index) rule else existing }
        }

    /** A typed cap, or `null` when it is not a whole number the server would accept. */
    private fun capOf(typed: String): Int? = typed.trim().toIntOrNull()?.takeIf { it in MIN_MAX_PINNED..MAX_MAX_PINNED }

    private companion object {
        /** Matches the other presenters, so a screen and the queries behind it stop together. */
        const val STOP_TIMEOUT_MILLIS = 5_000L
    }
}
