package ru.tinyops.turboist.nativeapp.navigation

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.filterNotNull
import javax.inject.Inject
import javax.inject.Singleton

/**
 * A task named by a link from outside, held until there is a screen to show it.
 *
 * A link reaches the app as an intent, and an intent can arrive at a moment when
 * nothing can answer it: on a cold start, or while nobody is signed in and the
 * only tree on screen is the sign-in one, which has no task screen in it at all.
 * Dropping it there would lose the link for good — the user tapped something and
 * the app opened somewhere else — so it waits here instead, and is followed the
 * moment the app's own graph appears.
 *
 * Exactly one link is held. A second replaces the first: both came from the same
 * person a moment apart, and the newer one is the one they are looking at. It is
 * cleared once followed, so putting the app away and coming back does not jump
 * to a task the user has already left.
 */
@Singleton
class PendingTaskLinks
    @Inject
    constructor() {
        private val pending = MutableStateFlow<Long?>(null)

        /** Each link waiting to be followed, by the server's id for the task. */
        fun pending(): Flow<Long> = pending.filterNotNull()

        /** Records a link that has arrived. */
        fun offer(serverId: Long) {
            pending.value = serverId
        }

        /** Says the link has been followed and need not be held. */
        fun followed() {
            pending.value = null
        }
    }
