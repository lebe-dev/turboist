package ru.tinyops.turboist.nativeapp.di

import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import ru.tinyops.turboist.nativeapp.account.ApiTokenControl
import ru.tinyops.turboist.nativeapp.account.HttpApiTokenControl
import ru.tinyops.turboist.nativeapp.account.HttpSessionControl
import ru.tinyops.turboist.nativeapp.account.HttpTwoFactorControl
import ru.tinyops.turboist.nativeapp.account.SessionControl
import ru.tinyops.turboist.nativeapp.account.TwoFactorControl
import javax.inject.Singleton

/**
 * The account's administrative surfaces, wired to HTTP and to nothing else.
 *
 * There is deliberately no cache, no store and no database binding here. Which
 * sessions are open, which tokens exist and whether a second factor is on are
 * answers that are only true at the instant the server gives them, and a
 * remembered one would be a confident lie about the security of the account.
 * Keeping the wiring this bare is what makes that visible at a glance.
 */
@Module
@InstallIn(SingletonComponent::class)
abstract class AccountModule {
    @Binds
    @Singleton
    abstract fun bindSessionControl(control: HttpSessionControl): SessionControl

    @Binds
    @Singleton
    abstract fun bindApiTokenControl(control: HttpApiTokenControl): ApiTokenControl

    @Binds
    @Singleton
    abstract fun bindTwoFactorControl(control: HttpTwoFactorControl): TwoFactorControl
}
