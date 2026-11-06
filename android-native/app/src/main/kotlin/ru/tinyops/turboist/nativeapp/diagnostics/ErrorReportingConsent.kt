package ru.tinyops.turboist.nativeapp.diagnostics

import android.content.Context
import dagger.hilt.android.qualifiers.ApplicationContext
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import javax.inject.Inject
import javax.inject.Singleton

/**
 * Whether the user has agreed to send error reports from this device.
 *
 * Kept in a plain preferences file rather than beside the device's other choices,
 * because a crash during start-up is the one worth hearing about most: the answer
 * has to be known before the first line of the application runs, and that file
 * can be read synchronously without blocking on a coroutine. The answer outlives
 * every session, like the other choices about the device. Absent means no.
 */
@Singleton
class ErrorReportingConsent
    @Inject
    constructor(
        @param:ApplicationContext context: Context,
    ) {
        private val preferences = context.getSharedPreferences(FILE, Context.MODE_PRIVATE)
        private val current = MutableStateFlow(preferences.getBoolean(KEY, false))

        val allowed: StateFlow<Boolean> = current.asStateFlow()

        fun set(allowed: Boolean) {
            preferences.edit().putBoolean(KEY, allowed).apply()
            current.value = allowed
        }

        private companion object {
            const val FILE = "error_reporting"
            const val KEY = "send_reports"
        }
    }
