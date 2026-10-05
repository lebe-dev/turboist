package ru.tinyops.turboist.nativeapp.e2e

import androidx.test.platform.app.InstrumentationRegistry

/**
 * Where the server is and who to sign in as, as handed to the suite when it is
 * launched.
 *
 * Two addresses, and the difference between them is the whole trick that makes
 * an offline test possible. [appBaseUrl] is the address the app dials, over the
 * device's own radios, so switching those off makes the server genuinely
 * unreachable to it. [controlBaseUrl] reaches the same server through the debug
 * bridge instead of the radios, so the checks — and the server-side edits that
 * have to happen *while the app is offline* — keep working when the app cannot
 * reach anything at all.
 *
 * Nothing is defaulted. A missing value means the suite was started by hand
 * against no server, and inventing an address would turn that into a confusing
 * failure several minutes later instead of a sentence now.
 */
data class HarnessSettings(
    val appBaseUrl: String,
    val controlBaseUrl: String,
    val username: String,
    val password: String,
) {
    companion object {
        const val APP_BASE_URL: String = "turboistAppBaseUrl"
        const val CONTROL_BASE_URL: String = "turboistControlBaseUrl"
        const val USERNAME: String = "turboistUsername"
        const val PASSWORD: String = "turboistPassword"

        fun fromInstrumentation(): HarnessSettings {
            val arguments = InstrumentationRegistry.getArguments()

            fun required(name: String): String =
                arguments.getString(name)?.takeIf { it.isNotBlank() }
                    ?: error(
                        "This suite drives a real server and was started without \"$name\". " +
                            "Run it with `just android-native-e2e`, which builds the server, " +
                            "starts it on a free port and passes the addresses in.",
                    )

            return HarnessSettings(
                appBaseUrl = required(APP_BASE_URL),
                controlBaseUrl = required(CONTROL_BASE_URL),
                username = required(USERNAME),
                password = required(PASSWORD),
            )
        }
    }
}
