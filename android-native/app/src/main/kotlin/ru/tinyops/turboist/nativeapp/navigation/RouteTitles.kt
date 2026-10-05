package ru.tinyops.turboist.nativeapp.navigation

import androidx.annotation.StringRes
import ru.tinyops.turboist.nativeapp.R
import kotlin.reflect.KClass

/**
 * The screen title for every route in the app.
 *
 * Drawer entries bring their own label, so they are folded in from the drawer
 * catalog rather than repeated here; only the routes that never appear in the
 * drawer — detail screens and the authentication flow — need an entry below.
 * A route missing from this map would render with no title, so a unit test
 * asserts the map covers the whole route set.
 */
object RouteTitles {
    private val drawerTitles: Map<KClass<*>, Int> =
        DrawerDestination.entries.associate { it.route::class to it.labelRes }

    private val detailTitles: Map<KClass<*>, Int> =
        mapOf(
            ProjectRoute::class to R.string.native_dest_project,
            LabelRoute::class to R.string.native_dest_label,
            ContextRoute::class to R.string.native_dest_context,
            PasskeysRoute::class to R.string.settings_passkeys_heading,
            SessionsRoute::class to R.string.settings_sessions_heading,
            ApiTokensRoute::class to R.string.settings_api_heading,
            TwoFactorRoute::class to R.string.settings_twofa_heading,
            TemplatesRoute::class to R.string.settings_templates_heading,
            UnsentChangesRoute::class to R.string.offline_unsentTitle,
            TaskRoute::class to R.string.native_dest_task,
            TaskLinkRoute::class to R.string.native_dest_task,
            ConnectRoute::class to R.string.native_dest_connect,
            SetupRoute::class to R.string.native_dest_setup,
            LoginRoute::class to R.string.native_dest_login,
        )

    /** Route type to title resource, drawer entries included. */
    val all: Map<KClass<*>, Int> = drawerTitles + detailTitles

    /** Title for a route type, or null when the type is not part of the graph. */
    @StringRes
    fun titleFor(routeClass: KClass<*>): Int? = all[routeClass]
}
