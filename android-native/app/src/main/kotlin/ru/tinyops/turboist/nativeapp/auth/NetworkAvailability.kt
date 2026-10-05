package ru.tinyops.turboist.nativeapp.auth

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import kotlinx.coroutines.flow.conflate
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Says when the device has a usable network again.
 *
 * A launch that could not reach the server keeps the stored session but has no
 * access token to show for it, and nothing in the app asks for one on its own.
 * Without a signal that the network came back, such a launch would render the
 * replica until the process is killed and started again somewhere with a
 * connection — which is precisely the situation the offline launch exists to
 * spare the user from.
 *
 * It reports transitions, not a state to draw. Whether the session has been
 * proved is the session layer's answer, and a screen that read connectivity
 * instead would claim the app is current the moment a phone joins a network
 * that cannot actually reach the server.
 */
fun interface NetworkAvailability {
    /** Emits once each time a network becomes usable. Never completes on its own. */
    fun whenAvailable(): Flow<Unit>
}

/**
 * The platform's own answer, from the default network callback.
 *
 * Emissions are conflated: a burst of transitions — a handover from mobile data
 * to a wifi network, say — is one thing to react to, not three. The collector
 * re-checks the session, and doing that per transition would spend the rotating
 * refresh token several times over for a single event.
 */
@Singleton
class ConnectivityNetworkAvailability
    @Inject
    constructor(
        @param:ApplicationContext private val context: Context,
    ) : NetworkAvailability {
        override fun whenAvailable(): Flow<Unit> =
            callbackFlow {
                val manager = context.getSystemService(ConnectivityManager::class.java)
                val callback =
                    object : ConnectivityManager.NetworkCallback() {
                        override fun onAvailable(network: Network) {
                            trySend(Unit)
                        }
                    }
                manager?.registerDefaultNetworkCallback(callback)
                awaitClose { manager?.unregisterNetworkCallback(callback) }
            }.conflate()
    }
