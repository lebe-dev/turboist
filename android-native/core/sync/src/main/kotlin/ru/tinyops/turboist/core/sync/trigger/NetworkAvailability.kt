package ru.tinyops.turboist.core.sync.trigger

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.util.Log
import kotlinx.coroutines.channels.awaitClose
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.callbackFlow
import java.util.concurrent.atomic.AtomicBoolean

/** Tells its collector each time the device gets a usable network back. */
fun interface NetworkAvailability {
    /**
     * Emits once per transition from no network to a network, and never for the
     * network the device already had when collection started — that one is not
     * news, and treating it as news would mean a sync every time anything started
     * watching.
     */
    fun regained(): Flow<Unit>
}

/**
 * [NetworkAvailability] read from the platform.
 *
 * A device that has been out of coverage holds every write the user made while
 * it was, and the moment coverage returns is the best one there is to send them:
 * the radio is already awake and the user is still standing where they made the
 * change. Waiting for the next background run instead would be up to a quarter
 * of an hour of the app looking wrong.
 */
class ConnectivityWatcher(
    private val context: Context,
) : NetworkAvailability {
    override fun regained(): Flow<Unit> =
        callbackFlow {
            val manager = context.getSystemService(ConnectivityManager::class.java)
            if (manager == null) {
                Log.w(
                    SyncScheduler.LOG_TAG,
                    "The platform reported no connectivity service; network changes are not watched",
                )
                awaitClose { }
                return@callbackFlow
            }
            val connected = AtomicBoolean(manager.hasUsableNetwork())
            val callback =
                object : ConnectivityManager.NetworkCallback() {
                    override fun onAvailable(network: Network) {
                        // A network arriving while one was already there is a
                        // handover, not a return: nothing was queued up waiting
                        // for it, so it is not a reason to go and ask the server.
                        if (connected.compareAndSet(false, true)) trySend(Unit)
                    }

                    override fun onLost(network: Network) {
                        if (manager.hasUsableNetwork()) return
                        connected.set(false)
                    }
                }
            manager.registerDefaultNetworkCallback(callback)
            awaitClose { manager.unregisterNetworkCallback(callback) }
        }

    private fun ConnectivityManager.hasUsableNetwork(): Boolean {
        val active = activeNetwork ?: return false
        val capabilities = getNetworkCapabilities(active) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }
}
