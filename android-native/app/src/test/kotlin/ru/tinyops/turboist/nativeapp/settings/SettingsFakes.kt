package ru.tinyops.turboist.nativeapp.settings

import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.LinkTitleRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest

/**
 * The settings writes, recorded instead of made.
 *
 * The three lists are kept apart on purpose: the claim the tests make is that
 * one kind of write never reaches the store of another, and a single "calls"
 * list would make that claim unaskable.
 */
class RecordingSettingsActions(
    var refuse: Boolean = false,
) : SettingsActions {
    val userPatches = mutableListOf<PatchUserSettingsRequest>()
    val autoLabelWrites = mutableListOf<List<AutoLabelRule>>()
    val suggestionWrites = mutableListOf<List<ProjectSuggestionRule>>()
    val linkTitleWrites = mutableListOf<LinkTitlesWrite>()

    override suspend fun patchUserSettings(edit: PatchUserSettingsRequest) {
        refuseIfAsked()
        userPatches += edit
    }

    override suspend fun putAutoLabels(rules: List<AutoLabelRule>) {
        refuseIfAsked()
        autoLabelWrites += rules
    }

    override suspend fun putProjectSuggestions(rules: List<ProjectSuggestionRule>) {
        refuseIfAsked()
        suggestionWrites += rules
    }

    override suspend fun putLinkTitles(
        enabled: Boolean?,
        template: String?,
        rules: List<LinkTitleRule>?,
    ) {
        refuseIfAsked()
        linkTitleWrites += LinkTitlesWrite(enabled, template, rules)
    }

    private fun refuseIfAsked() {
        if (refuse) throw IllegalStateException("the server refused this write")
    }
}

/** One link-title write as it was asked for, nulls meaning "left alone". */
data class LinkTitlesWrite(
    val enabled: Boolean?,
    val template: String?,
    val rules: List<LinkTitleRule>?,
)

/** The device's own choices, recorded instead of stored. */
class RecordingDeviceOptionActions : DeviceOptionActions {
    var theme: ThemeChoice? = null
    var syncOnMetered: Boolean? = null
    var sendErrorReports: Boolean? = null

    override suspend fun setTheme(theme: ThemeChoice) {
        this.theme = theme
    }

    override suspend fun setSyncOnMetered(allowed: Boolean) {
        syncOnMetered = allowed
    }

    override suspend fun setSendErrorReports(allowed: Boolean) {
        sendErrorReports = allowed
    }
}

/** The connection, with both destructive actions recorded rather than taken. */
class RecordingServerConnection(
    private val address: String = "https://turboist.example/",
    var unsent: Int = 0,
) : ServerConnection {
    var forgotten: Int = 0
    var cleared: Int = 0

    override suspend fun address(): String = address

    override suspend fun unsentChangeCount(): Int = unsent

    override suspend fun forgetServer() {
        forgotten++
    }

    override suspend fun clearLocalData() {
        cleared++
    }
}
