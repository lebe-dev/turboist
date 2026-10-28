package ru.tinyops.turboist.nativeapp.shell

import androidx.compose.runtime.Stable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf

/**
 * What a screen can ask of the shell's chrome around it.
 *
 * The shell draws the capture button and the navigation bar over every screen,
 * and a screen that takes over the bottom edge for a moment — a list putting up
 * its selection toolbar — has to be able to ask for them to step aside. The
 * request is state rather than a callback so it lasts exactly as long as the
 * screen holds it, and is dropped with the screen.
 */
@Stable
class ShellChrome {
    /** True while a screen has tasks picked and its toolbar owns the bottom edge. */
    var selectionActive: Boolean by mutableStateOf(false)
}

/** The shell's chrome, or `null` when a screen is composed without a shell around it. */
val LocalShellChrome = staticCompositionLocalOf<ShellChrome?> { null }
