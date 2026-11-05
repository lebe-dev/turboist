package ru.tinyops.turboist.baselineprofile

import android.widget.EditText
import androidx.benchmark.macro.MacrobenchmarkScope
import androidx.test.uiautomator.By
import androidx.test.uiautomator.Direction
import androidx.test.uiautomator.UiObject2
import androidx.test.uiautomator.Until

/** The id of the build under test, as read off its APK by the build. */
internal val targetAppId: String
    get() =
        androidx.test.platform.app.InstrumentationRegistry.getArguments().getString("targetAppId")
            ?: error("The build did not say which application is under test.")

private const val SCREEN_TIMEOUT_MILLIS = 30_000L
private const val FIRST_SYNC_TIMEOUT_MILLIS = 120_000L
private const val FIELD_TIMEOUT_MILLIS = 15_000L
private const val GESTURE_MARGIN_DIVISOR = 5
private const val FLINGS = 3

private const val CONNECT_BUTTON = "Connect"
private const val SIGN_IN_BUTTON = "Sign in"
private const val INBOX_TAB = "Inbox"
private const val TODAY_TAB = "Today"
private const val WEEK_TAB = "Week"
private const val PROJECTS_TAB = "Projects"

/**
 * Fixes the app's language to the one these selectors are written in.
 *
 * The device's own language is whatever the machine running this happens to use,
 * and a journey that finds "Inbox" on one phone and nothing on another would
 * report it as a timeout on a screen that is working perfectly.
 */
internal fun MacrobenchmarkScope.useEnglish() {
    device.executeShellCommand("cmd locale set-app-locales $packageName --locales en")
}

/** Launches the app and leaves it on the signed-in Today list, however far along it already was. */
internal fun MacrobenchmarkScope.launchSignedIn() {
    BenchmarkWorkspace.ensureSeeded()
    useEnglish()
    pressHome()
    startActivityAndWait()

    val landed =
        device.wait(
            Until.findObject(By.text(BenchmarkWorkspace.TODAY_ANCHOR).pkg(packageName)),
            SCREEN_TIMEOUT_MILLIS / 2,
        )
    if (landed != null) return

    signIn()
    awaitText(BenchmarkWorkspace.TODAY_ANCHOR, FIRST_SYNC_TIMEOUT_MILLIS)
}

private fun MacrobenchmarkScope.signIn() {
    // Looked for by its text alone: both buttons stay disabled until their fields are filled.
    val connect = device.wait(Until.findObject(By.text(CONNECT_BUTTON)), SCREEN_TIMEOUT_MILLIS)
    checkNotNull(connect) { "Neither the Today list nor the connect screen appeared." }
    field(0).text = BenchmarkWorkspace.serverUrl
    awaitButton(CONNECT_BUTTON).click()

    val signIn = device.wait(Until.findObject(By.text(SIGN_IN_BUTTON)), SCREEN_TIMEOUT_MILLIS)
    checkNotNull(signIn) { "The sign-in screen did not appear after connecting to the server." }
    field(0).text = BenchmarkWorkspace.USERNAME
    field(1).text = BenchmarkWorkspace.PASSWORD
    awaitButton(SIGN_IN_BUTTON).click()
}

private fun MacrobenchmarkScope.field(index: Int): UiObject2 {
    device.wait(Until.hasObject(By.clazz(EditText::class.java)), FIELD_TIMEOUT_MILLIS)
    return device.findObjects(By.clazz(EditText::class.java))[index]
}

/** A button with this label once it can be pressed; the label is a child of the node that takes the press. */
private fun MacrobenchmarkScope.awaitButton(label: String): UiObject2 {
    val button = By.clickable(true).enabled(true).hasChild(By.text(label))
    return checkNotNull(device.wait(Until.findObject(button), FIELD_TIMEOUT_MILLIS)) {
        "The \"$label\" button never became available."
    }
}

private fun MacrobenchmarkScope.awaitText(
    text: String,
    timeout: Long = SCREEN_TIMEOUT_MILLIS,
): UiObject2 =
    checkNotNull(device.wait(Until.findObject(By.text(text).pkg(packageName)), timeout)) {
        "\"$text\" never appeared on screen."
    }

/** Selects a tab of the bottom bar, whose label stays on screen on every top-level destination. */
internal fun MacrobenchmarkScope.openTab(label: String) {
    awaitText(label).click()
    device.waitForIdle()
}

/** Scrolls whatever list is on screen down and back up again. */
internal fun MacrobenchmarkScope.flingList() {
    val list =
        checkNotNull(device.wait(Until.findObject(By.scrollable(true).pkg(packageName)), SCREEN_TIMEOUT_MILLIS)) {
            "The screen holds no scrollable list."
        }
    // Keeps the gesture away from the screen edges, which the system claims for itself.
    list.setGestureMargin(device.displayWidth / GESTURE_MARGIN_DIVISOR)
    repeat(FLINGS) {
        list.fling(Direction.DOWN)
        device.waitForIdle()
    }
    repeat(FLINGS) {
        list.fling(Direction.UP)
        device.waitForIdle()
    }
}

/** Every screen a person spends the day on, with its list scrolled. */
internal fun MacrobenchmarkScope.walkTheApp() {
    flingList()

    openTab(INBOX_TAB)
    awaitText(BenchmarkWorkspace.INBOX_ANCHOR)
    flingList()

    openTab(WEEK_TAB)
    flingList()

    openTab(PROJECTS_TAB)
    awaitText(BenchmarkWorkspace.FIRST_PROJECT).click()
    awaitText(BenchmarkWorkspace.TODAY_ANCHOR)
    flingList()
    device.pressBack()

    openTab(TODAY_TAB)
    awaitText(BenchmarkWorkspace.TODAY_ANCHOR).click()
    // The detail screen, its description rendered: the one place Markdown is parsed.
    awaitText("Notes")
    device.pressBack()
    awaitText(BenchmarkWorkspace.TODAY_ANCHOR)
}

/** Moves to the longest list and leaves it at the top, ready to be scrolled. */
internal fun MacrobenchmarkScope.openLongList() {
    openTab(TODAY_TAB)
    awaitText(BenchmarkWorkspace.TODAY_ANCHOR)
}
