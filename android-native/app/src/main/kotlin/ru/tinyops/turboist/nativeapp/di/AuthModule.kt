package ru.tinyops.turboist.nativeapp.di

import android.security.NetworkSecurityPolicy
import dagger.Binds
import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.auth.AuthGateway
import ru.tinyops.turboist.nativeapp.auth.CleartextPolicy
import ru.tinyops.turboist.nativeapp.auth.ConnectivityNetworkAvailability
import ru.tinyops.turboist.nativeapp.auth.DataStoreServerAddressStore
import ru.tinyops.turboist.nativeapp.auth.HttpAuthGateway
import ru.tinyops.turboist.nativeapp.auth.KeystoreRefreshTokenStore
import ru.tinyops.turboist.nativeapp.auth.LocalReplica
import ru.tinyops.turboist.nativeapp.auth.NetworkAvailability
import ru.tinyops.turboist.nativeapp.auth.RefreshTokenStore
import ru.tinyops.turboist.nativeapp.auth.RoomLocalReplica
import ru.tinyops.turboist.nativeapp.auth.ServerAddressStore
import javax.inject.Singleton

/**
 * What the session layer talks to, each thing behind an interface.
 *
 * The seams are what make the session rules testable: the launch decision tree,
 * token rotation and the sign-out wipe are all exercised against hand-written
 * stand-ins, with no server, no keystore and no database in sight.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class AuthModule {
    @Binds
    @Singleton
    abstract fun bindAuthGateway(gateway: HttpAuthGateway): AuthGateway

    @Binds
    @Singleton
    abstract fun bindRefreshTokenStore(store: KeystoreRefreshTokenStore): RefreshTokenStore

    @Binds
    @Singleton
    abstract fun bindServerAddressStore(store: DataStoreServerAddressStore): ServerAddressStore

    @Binds
    @Singleton
    abstract fun bindLocalReplica(replica: RoomLocalReplica): LocalReplica

    @Binds
    @Singleton
    abstract fun bindNetworkAvailability(availability: ConnectivityNetworkAvailability): NetworkAvailability

    companion object {
        /**
         * Whether the connect screen may accept a plain-HTTP address.
         *
         * The platform is asked rather than told: it permits cleartext only where
         * the manifest opted in, and only the debug manifest does. Following its
         * answer is what keeps the screen's rule and the transport's behaviour
         * from ever disagreeing — a shipping build refuses such an address here
         * *and* could not send it anyway.
         */
        @Provides
        @Singleton
        fun cleartextPolicy(): CleartextPolicy =
            CleartextPolicy(allowed = NetworkSecurityPolicy.getInstance().isCleartextTrafficPermitted)
    }
}
