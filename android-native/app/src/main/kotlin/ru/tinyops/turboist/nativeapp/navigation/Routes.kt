package ru.tinyops.turboist.nativeapp.navigation

// Every destination the app can navigate to, one type per screen.
//
// The routes are Kotlin types rather than format strings so a destination and
// its arguments are checked by the compiler: `navigate(ProjectRoute(7))` cannot
// drift from the composable that reads `ProjectRoute.projectId`.
//
// The set mirrors the web client's URL space one for one, which is what makes a
// link opened on the phone land on the same screen the browser would show.

import kotlinx.serialization.Serializable

// --- Authentication ------------------------------------------------------

/** Server URL entry. The first screen of a fresh install. */
@Serializable
data object ConnectRoute

/** First-run account creation, offered when the server has no user yet. */
@Serializable
data object SetupRoute

/** Username and password sign-in, including the second-factor step. */
@Serializable
data object LoginRoute

// --- Task views ----------------------------------------------------------

@Serializable
data object TodayRoute

@Serializable
data object TomorrowRoute

@Serializable
data object WeekRoute

@Serializable
data object NextWeekRoute

@Serializable
data object InboxRoute

@Serializable
data object CompletedRoute

@Serializable
data object TroikiRoute

@Serializable
data object SearchRoute

@Serializable
data object SettingsRoute

/**
 * The reusable blueprints: the templates the user keeps, and the editor that
 * writes one. Reached from settings rather than from the drawer — a template is
 * a setting for how work gets written down, not a place work happens.
 */
@Serializable
data object TemplatesRoute

/**
 * The account's passkeys: which devices can sign in without a password, and the
 * enrolment that adds one. Reached from settings rather than from the drawer —
 * it is a detail of the account, not a place work happens.
 */
@Serializable
data object PasskeysRoute

/**
 * The ways into the account that are open right now: which devices and browsers
 * hold a session, and how to close one. Reached from settings, and never
 * replicated — the list is only true at the instant the server gives it.
 */
@Serializable
data object SessionsRoute

/**
 * The long-lived tokens external tools authenticate with. Reached from settings
 * for the same reason as the session list, and read live for the same one.
 */
@Serializable
data object ApiTokensRoute

/**
 * The time-based second factor: whether the account asks for a code, and turning
 * that on or off. Reached from settings; nothing about it is stored on the
 * device, including the secret an enrolment issues.
 */
@Serializable
data object TwoFactorRoute

/**
 * What this device has changed and the server has not taken: the queue that is
 * still going out, and the pile it refused. Reached from settings rather than
 * from the drawer — it is a place the user goes when something is wrong, not one
 * of the places work happens.
 */
@Serializable
data object UnsentChangesRoute

// --- Collections and their detail screens --------------------------------

@Serializable
data object ProjectsRoute

// One project and its board, named the way the device holds it.
//
// The device's own id addresses every project in the replica, the ones started
// while offline included, so a project created a moment ago opens like any
// other. Addressing it by the server's id instead would leave exactly those
// projects unreachable until the queue drained.
@Serializable
data class ProjectRoute(val projectLocalId: Long)

@Serializable
data object LabelsRoute

// One label and the work carrying it, named the way the device holds it, for the
// same reason a project is: a label created while offline has no server id yet,
// and addressing it by one would leave exactly those labels unreachable until
// the queue drained.
@Serializable
data class LabelRoute(val labelLocalId: Long)

// One context, named the way the device holds it, for the same reason a project
// is.
@Serializable
data class ContextRoute(val contextLocalId: Long)

// A single task, named the way the device holds it.
//
// The device's own id addresses every task in the replica, including the ones
// written down while offline that the server has never heard of — so a task
// created a moment ago can be opened like any other. Anything arriving from
// outside the app carries the server's id instead and comes in through
// [TaskLinkRoute].
@Serializable
data class TaskRoute(val taskLocalId: Long)

// A task named the way the server names it.
//
// This is the id the web client puts in its `/task/{id}` URL, so a link copied
// out of a browser opens the same task here. It is a destination of its own
// rather than a second field on [TaskRoute] because the two ids answer different
// questions: one always names a row on this device, the other names a row the
// device may not have replicated yet. The screen behind both is the same, and it
// is what translates.
@Serializable
data class TaskLinkRoute(val serverId: Long)
