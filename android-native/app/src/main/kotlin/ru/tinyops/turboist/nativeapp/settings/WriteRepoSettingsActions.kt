package ru.tinyops.turboist.nativeapp.settings

import android.content.Context
import android.content.pm.PackageManager
import dagger.hilt.android.qualifiers.ApplicationContext
import ru.tinyops.turboist.core.model.AutoLabelRule
import ru.tinyops.turboist.core.model.ProjectSuggestionRule
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.dto.PatchUserSettingsRequest
import ru.tinyops.turboist.core.sync.write.SettingsWriteRepo
import ru.tinyops.turboist.nativeapp.auth.LocalReplica
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.sync.MeteredSyncPolicy
import ru.tinyops.turboist.nativeapp.sync.SyncScheduler
import javax.inject.Inject
import javax.inject.Singleton

/**
 * The settings screen's writes, made against the shared write path.
 *
 * Nothing is decided here. Each call applies its change to the replica and
 * queues it for the server in one transaction, which is what makes a preference
 * changed on a train appear immediately and arrive later.
 */
@Singleton
class WriteRepoSettingsActions
    @Inject
    constructor(
        private val settings: SettingsWriteRepo,
    ) : SettingsActions {
        override suspend fun patchUserSettings(edit: PatchUserSettingsRequest) {
            settings.patchUserSettings(edit)
        }

        override suspend fun putAutoLabels(rules: List<AutoLabelRule>) {
            settings.putAutoLabels(rules)
        }

        override suspend fun putProjectSuggestions(rules: List<ProjectSuggestionRule>) {
            settings.putProjectSuggestions(rules)
        }
    }

/** The device's own choices, stored and applied. */
@Singleton
class StoredDeviceOptionActions
    @Inject
    constructor(
        private val store: DeviceOptionsStore,
        private val metered: MeteredSyncPolicy,
    ) : DeviceOptionActions {
        override suspend fun setTheme(theme: ThemeChoice) {
            store.setTheme(theme)
        }

        override suspend fun setSyncOnMetered(allowed: Boolean) {
            store.setSyncOnMetered(allowed)
            // Stored first, applied second: a process killed between the two comes
            // back and reads the stored answer, while the reverse order would
            // leave a scheduled job obeying a choice nothing remembers.
            metered.apply(allowed)
        }
    }

/** The connection as the session layer and the replica actually hold it. */
@Singleton
class SessionServerConnection
    @Inject
    constructor(
        private val session: SessionManager,
        private val serverUrl: ServerUrl,
        private val replica: LocalReplica,
        private val sync: SyncScheduler,
    ) : ServerConnection {
        override suspend fun address(): String = serverUrl.value?.toString().orEmpty()

        override suspend fun unsentChangeCount(): Int = replica.unsentChangeCount()

        override suspend fun forgetServer() {
            session.disconnect()
        }

        override suspend fun clearLocalData() {
            replica.wipe()
            // Asked for straight away rather than left to the schedule: the user
            // is looking at an app that has just emptied itself, and "it will
            // fill back in eventually" is indistinguishable from broken.
            sync.requestSyncNow()
        }
    }

/**
 * What this build calls itself, read from the package the system installed.
 *
 * Read from the platform rather than compiled in, so it cannot drift from the
 * artifact: the version is stamped onto the package at build time from the
 * repository's own version file, together with the commit when the machine
 * building it supplied one.
 */
@Singleton
class PackageAppRelease
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : AppRelease {
        override fun versionName(): String =
            try {
                context.packageManager.getPackageInfo(context.packageName, 0).versionName.orEmpty()
            } catch (e: PackageManager.NameNotFoundException) {
                // A package that cannot find itself is not a state worth crashing
                // an otherwise working settings screen over.
                android.util.Log.w("TurboistSettings", "This build could not read its own version", e)
                ""
            }
    }
