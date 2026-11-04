package ru.tinyops.turboist.nativeapp.navigation

import ru.tinyops.turboist.nativeapp.settings.SettingsSection
import kotlin.reflect.KClass

/** The destination each settings section is shown at. */
val SettingsSection.route: Any
    get() =
        when (this) {
            SettingsSection.GENERAL -> SettingsGeneralRoute
            SettingsSection.WORKSPACE -> SettingsWorkspaceRoute
            SettingsSection.RULES -> SettingsRulesRoute
            SettingsSection.SECURITY -> SettingsSecurityRoute
            SettingsSection.DEVICE -> SettingsDeviceRoute
            SettingsSection.ABOUT -> SettingsAboutRoute
        }

/**
 * The destinations that are reached only through settings.
 *
 * Every one of them is a step down from a place rather than a place, so the bar
 * over it offers the way back up where a top-level screen offers the drawer. A
 * screen reached by a menu button on every level would leave the user with no
 * visible way out but the system gesture.
 */
val SettingsChildRoutes: Set<KClass<*>> =
    SettingsSection.entries.map { it.route::class }.toSet() +
        setOf(
            PasskeysRoute::class,
            SessionsRoute::class,
            ApiTokensRoute::class,
            TwoFactorRoute::class,
            TemplatesRoute::class,
            UnsentChangesRoute::class,
        )
