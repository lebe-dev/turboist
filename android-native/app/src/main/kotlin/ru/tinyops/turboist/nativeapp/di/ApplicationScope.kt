package ru.tinyops.turboist.nativeapp.di

import javax.inject.Qualifier

/**
 * A coroutine scope that lives as long as the process.
 *
 * For work that belongs to the app rather than to whatever screen started it —
 * resolving the stored session at launch, finishing a sign-out. Tying such work
 * to a screen's scope would cancel it the moment the screen went away, half
 * done.
 */
@Qualifier
@Retention(AnnotationRetention.BINARY)
annotation class ApplicationScope
