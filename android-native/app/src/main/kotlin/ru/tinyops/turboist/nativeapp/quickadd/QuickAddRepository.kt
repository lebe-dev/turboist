package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.map
import ru.tinyops.turboist.core.database.dao.LabelDao
import ru.tinyops.turboist.core.database.dao.ProjectDao
import ru.tinyops.turboist.core.database.dao.SettingsDao
import ru.tinyops.turboist.core.database.entity.LabelRow
import ru.tinyops.turboist.core.database.entity.toDomain
import ru.tinyops.turboist.core.model.AppSettings
import ru.tinyops.turboist.core.model.Label
import ru.tinyops.turboist.core.model.Project
import ru.tinyops.turboist.core.model.ProjectStatus
import ru.tinyops.turboist.core.network.TurboistJson
import ru.tinyops.turboist.core.network.dto.AppSettingsDto
import ru.tinyops.turboist.core.network.mapping.toAppSettings
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Everything the capture surface offers to choose from.
 *
 * Read as one value because the three answer one question together — where can
 * this go, what can it be tagged with, and what will the installation do to it
 * on its own — and a sheet that saw them at three different instants could offer
 * a label it then failed to find.
 */
data class QuickAddWorkspace(
    val projects: List<Project> = emptyList(),
    val labels: List<Label> = emptyList(),
    val appSettings: AppSettings = AppSettings(),
)

/**
 * What the capture surface reads.
 *
 * A standing query over the replica, like every other screen: capture works with
 * no network at all, and a project created on another device appears in the
 * picker the moment it is replicated rather than when the sheet is next opened.
 */
@Singleton
class QuickAddRepository
    @Inject
    constructor(
        private val projects: ProjectDao,
        private val labels: LabelDao,
        private val settings: SettingsDao,
    ) {
        fun observeWorkspace(): Flow<QuickAddWorkspace> =
            combine(
                projects.observeAll(),
                labels.observeAll(),
                observeAppSettings(),
            ) { projectRows, labelRows, appSettings ->
                QuickAddWorkspace(
                    projects = projectRows.map { it.toDomain() }.filter(::isOffered).sortedWith(PICKER_ORDER),
                    labels = labelRows.map(LabelRow::toDomain),
                    appSettings = appSettings,
                )
            }

        /**
         * The installation's title rules as last replicated.
         *
         * A document this build cannot read falls back to no rules rather than
         * failing: a newer server must not be able to stop the user writing
         * something down. The cost of the fallback is that a label the rules
         * would have attached is attached by the server instead, one sync later.
         */
        private fun observeAppSettings(): Flow<AppSettings> =
            settings.observeAppSettings().map { row ->
                val payload = row?.payload ?: return@map AppSettings()
                runCatching { TurboistJson.decodeFromString(AppSettingsDto.serializer(), payload) }
                    .map { it.toAppSettings() }
                    .getOrElse { AppSettings() }
            }

        /**
         * Whether a project is somewhere new work can go.
         *
         * A finished project is not: filing into it would reopen a decision the
         * user has already made. Everything else is offered, archived projects
         * included — an archive is out of the way, not closed.
         */
        private fun isOffered(project: Project): Boolean = project.status != ProjectStatus.COMPLETED

        private companion object {
            /**
             * The order the picker lists projects in: pinned first, longest-pinned
             * of those first, then everything else by title regardless of case.
             * The same order the web client's picker uses, so the two lists read
             * the same way.
             */
            val PICKER_ORDER: Comparator<Project> =
                compareByDescending<Project> { it.isPinned }
                    .thenBy { if (it.isPinned) it.pinnedAt ?: Long.MAX_VALUE else 0L }
                    .thenBy(String.CASE_INSENSITIVE_ORDER) { it.title }
        }
    }
