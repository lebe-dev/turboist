package ru.tinyops.turboist.nativeapp.settings

import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest

/**
 * The writes the settings screen makes against the server's two preference
 * documents.
 *
 * They are separate calls rather than one "save settings", and that separation
 * is the point: the user's own preferences and the installation's rules are
 * different documents with different owners, and a port that took both at once
 * would make writing one while meaning the other a typo away.
 */
interface SettingsActions {
    /**
     * Changes some of the user's own preferences.
     *
     * The request carries only the keys the user actually changed. Sending a full
     * snapshot instead would hand the server this build's idea of every
     * preference — including the ones it has never heard of, which it would send
     * back as absent and so destroy.
     */
    suspend fun patchUserSettings(edit: PatchUserSettingsRequest)

    /**
     * Replaces the installation's automatic labelling rules, whole.
     *
     * The endpoint takes the entire list, so there is no such thing as adding one
     * rule; pretending otherwise would mean rebuilding the list at send time from
     * a state that has since moved on.
     */
    suspend fun putAutoLabels(rules: List<AutoLabelRule>)

    /** Replaces the installation's project suggestions, whole, for the same reason. */
    suspend fun putProjectSuggestions(rules: List<ProjectSuggestionRule>)
}

/** The device's own choices, changed. */
interface DeviceOptionActions {
    suspend fun setTheme(theme: ThemeChoice)

    /**
     * Records whether the background catch-up may spend mobile data, and applies
     * it to the jobs already scheduled — a constraint is fixed when a job is
     * enqueued, so a stored answer nobody re-applied would not take effect until
     * the next sign-in.
     */
    suspend fun setSyncOnMetered(allowed: Boolean)
}

/**
 * What this device is connected to, and the two ways of letting go of it.
 *
 * Both are destructive and both are asked about first, which is why the count of
 * what has not reached the server is part of this port rather than something the
 * screen infers: "some changes will be lost" is not a thing a person can weigh.
 */
interface ServerConnection {
    /** The address this device talks to, or an empty string before it has ever been connected. */
    suspend fun address(): String

    /** How many local changes the server has not accepted — queued and refused alike. */
    suspend fun unsentChangeCount(): Int

    /**
     * Points the app at nothing.
     *
     * The session ends, the replica is emptied and the address is forgotten, so
     * the app comes back at the screen that asks which server to talk to. It is
     * the only honest way to change servers: a replica is a copy of one
     * installation's data, and carrying it across to another would mix two
     * workspaces that share nothing but id numbers.
     */
    suspend fun forgetServer()

    /**
     * Empties the on-device copy and asks for it again.
     *
     * The session stays, so this is not a sign-out: it is the repair for a
     * replica that has gone wrong. Everything the server already holds comes
     * back on the next catch-up; everything it has not accepted yet does not,
     * which is why the count above is shown before this runs.
     */
    suspend fun clearLocalData()
}

/** What this build calls itself, for a screen that has to say so. */
fun interface AppRelease {
    /**
     * The version this build carries, with the commit it was made from when the
     * machine that built it supplied one.
     */
    fun versionName(): String
}
