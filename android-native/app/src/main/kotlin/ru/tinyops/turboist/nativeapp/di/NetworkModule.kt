package ru.tinyops.turboist.nativeapp.di

import dagger.Module
import dagger.Provides
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.core.network.ServerUrl
import ru.tinyops.turboist.core.network.TurboistNetwork
import ru.tinyops.turboist.core.network.api.ApiTokenApi
import ru.tinyops.turboist.core.network.api.AuthApi
import ru.tinyops.turboist.core.network.api.ContextApi
import ru.tinyops.turboist.core.network.api.LabelApi
import ru.tinyops.turboist.core.network.api.PasskeyApi
import ru.tinyops.turboist.core.network.api.ProjectApi
import ru.tinyops.turboist.core.network.api.SessionApi
import ru.tinyops.turboist.core.network.api.SettingsApi
import ru.tinyops.turboist.core.network.api.SyncApi
import ru.tinyops.turboist.core.network.api.TaskApi
import ru.tinyops.turboist.core.network.api.TemplateApi
import ru.tinyops.turboist.core.network.api.TotpApi
import ru.tinyops.turboist.core.network.auth.AccessTokenRefresher
import ru.tinyops.turboist.core.network.auth.AccessTokenSource
import ru.tinyops.turboist.nativeapp.auth.SessionManager
import ru.tinyops.turboist.nativeapp.auth.SessionTokens
import javax.inject.Provider
import javax.inject.Singleton

/**
 * The app's single HTTP stack.
 *
 * One client, shared by every endpoint group, is a correctness requirement
 * rather than a tidiness one: the token refresh is single-flight *within* a
 * client, and two clients would mean two refreshes racing to spend the same
 * rotating token — which the server reads as a stolen credential and answers by
 * killing the session.
 */
@Module
@InstallIn(SingletonComponent::class)
object NetworkModule {
    /**
     * The address holder. Empty at start-up: there is no compiled-in host, and
     * the connect screen fills it in — which is why the stack reads it per
     * request instead of being built around it.
     */
    @Provides
    @Singleton
    fun serverUrl(): ServerUrl = ServerUrl()

    @Provides
    @Singleton
    fun accessTokenSource(tokens: SessionTokens): AccessTokenSource = tokens

    /**
     * The repair hook the HTTP layer calls when an access token has aged out.
     *
     * Taken as a [Provider] on purpose: the session layer needs the HTTP stack to
     * talk to the server, and the HTTP stack needs the session layer to repair
     * itself. Resolving one of the two lazily is what turns that into a working
     * pair instead of a construction cycle.
     */
    @Provides
    @Singleton
    fun accessTokenRefresher(session: Provider<SessionManager>): AccessTokenRefresher =
        AccessTokenRefresher { expired -> session.get().renewAccessToken(expired) }

    @Provides
    @Singleton
    fun network(
        serverUrl: ServerUrl,
        tokens: AccessTokenSource,
        refresher: AccessTokenRefresher,
    ): TurboistNetwork = TurboistNetwork.create(serverUrl = serverUrl, tokens = tokens, refresher = refresher)

    @Provides
    fun authApi(network: TurboistNetwork): AuthApi = network.auth

    @Provides
    fun syncApi(network: TurboistNetwork): SyncApi = network.sync

    @Provides
    fun taskApi(network: TurboistNetwork): TaskApi = network.tasks

    @Provides
    fun projectApi(network: TurboistNetwork): ProjectApi = network.projects

    @Provides
    fun contextApi(network: TurboistNetwork): ContextApi = network.contexts

    @Provides
    fun labelApi(network: TurboistNetwork): LabelApi = network.labels

    @Provides
    fun templateApi(network: TurboistNetwork): TemplateApi = network.templates

    @Provides
    fun settingsApi(network: TurboistNetwork): SettingsApi = network.settings

    @Provides
    fun passkeyApi(network: TurboistNetwork): PasskeyApi = network.passkeys

    /**
     * The account's administrative endpoints. Nothing they return is replicated:
     * what they report is only true at the instant it is asked for.
     */
    @Provides
    fun sessionApi(network: TurboistNetwork): SessionApi = network.sessions

    @Provides
    fun apiTokenApi(network: TurboistNetwork): ApiTokenApi = network.apiTokens

    @Provides
    fun totpApi(network: TurboistNetwork): TotpApi = network.totp
}
