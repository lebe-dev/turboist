package ru.tinyops.turboist.baselineprofile

import androidx.benchmark.macro.junit4.BaselineProfileRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * Records the code a person actually runs, for the runtime to compile ahead of time.
 *
 * Two recordings, because they answer two questions. What launching touches also
 * decides how the code is laid out on disk, so only launching belongs in it; the
 * walk through the screens is what removes the stutter from a first scroll, and
 * mixing it in would move code nobody needs at launch into the part read at launch.
 */
@RunWith(AndroidJUnit4::class)
class BaselineProfileGenerator {
    @get:Rule
    val rule = BaselineProfileRule()

    @Test
    fun launching() =
        rule.collect(packageName = targetAppId, includeInStartupProfile = true) {
            launchSignedIn()
        }

    @Test
    fun usingTheApp() =
        rule.collect(packageName = targetAppId, includeInStartupProfile = false) {
            launchSignedIn()
            walkTheApp()
        }
}
