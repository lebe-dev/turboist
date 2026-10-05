package ru.tinyops.turboist.nativeapp.settings

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.SettingsDao
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.UserSettings
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.dto.UserSettingsDto
import ru.tinyops.turboist.core.network.mapping.toAppSettings
import ru.tinyops.turboist.core.network.mapping.toUserSettings
import javax.inject.Inject
import javax.inject.Singleton

/**
 * What the settings screen reads.
 *
 * Like every other screen in the app it reads the replica rather than fetching:
 * a preference changed on another device arrives because the row behind it
 * changed, and the screen opens with the last known answer when there is no
 * network at all.
 *
 * The two preference documents are read separately and never merged. They have
 * different owners — one is the user's own taste, the other is a rule the whole
 * installation obeys — and joining them into a single object here is exactly the
 * mistake that would let a screen write one while meaning the other.
 */
@Singleton
class SettingsRepository
    @Inject
    constructor(
        private val settings: SettingsDao,
        private val labels: LabelDao,
        private val projects: ProjectDao,
    ) {
        /**
         * The user's own preferences as last replicated.
         *
         * A document this build cannot read falls back to the product's defaults
         * rather than failing: a preference blob written by a newer server must
         * not be able to blank the screen that edits it.
         */
        fun observeUserSettings(): Flow<UserSettings> =
            settings.observeUserSettings().map { row ->
                val payload = row?.payload ?: return@map UserSettings()
                runCatching { TurboistJson.decodeFromString(UserSettingsDto.serializer(), payload) }
                    .map { it.toUserSettings() }
                    .getOrElse { UserSettings() }
            }

        /** The installation's rules as last replicated. Absent before a first sync, which means no rules. */
        fun observeAppSettings(): Flow<AppSettings> =
            settings.observeAppSettings().map { row ->
                val payload = row?.payload ?: return@map AppSettings()
                runCatching { TurboistJson.decodeFromString(AppSettingsDto.serializer(), payload) }
                    .map { it.toAppSettings() }
                    .getOrElse { AppSettings() }
            }

        /** The labels a preference or a rule can name. */
        fun observeLabels(): Flow<List<Label>> = labels.observeAll().map { rows -> rows.map { it.toDomain() } }

        /** The projects a suggestion rule can offer. */
        fun observeProjects(): Flow<List<Project>> = projects.observeAll().map { rows -> rows.map { it.toDomain() } }
    }
