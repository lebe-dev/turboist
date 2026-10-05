package ru.tinyops.turboist.nativeapp.quickadd

import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.filterNotNull
import javax.inject.Inject
import javax.inject.Singleton

/**
 * A capture asked for from outside, held until something can show it.
 *
 * A share or a launcher shortcut reaches the app as an intent, which arrives
 * before the sheet that answers it exists — sometimes before the user is even
 * signed in. Parking the request here means the activity does not have to know
 * what state the app is in, and the capture surface does not have to know about
 * intents.
 *
 * Exactly one request is held. A second one replaces the first: both came from
 * the same person a moment apart, and the newer one is the one they are looking
 * at. It is cleared once shown, so putting the app away and coming back does not
 * re-open a sheet the user already dealt with.
 */
@Singleton
class QuickAddRequests
    @Inject
    constructor() {
        private val pending = MutableStateFlow<QuickAddRequest?>(null)

        /** Each capture asked for, once it has been asked for. */
        fun pending(): Flow<QuickAddRequest> = pending.filterNotNull()

        /** Records what the user asked to capture. */
        fun offer(request: QuickAddRequest) {
            pending.value = request
        }

        /** Says the request has been put in front of the user and need not be held. */
        fun consumed() {
            pending.value = null
        }
    }
