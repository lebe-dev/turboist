package ru.tinyops.turboist.baselineprofile

import androidx.benchmark.macro.BaselineProfileMode
import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.FrameTimingMetric
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/** Scrolling the longest list, the moment a slow frame is most visible. */
@RunWith(AndroidJUnit4::class)
class ScrollBenchmark {
    @get:Rule
    val rule = MacrobenchmarkRule()

    @Test
    fun withoutProfile() = scroll(CompilationMode.None())

    @Test
    fun withShippedProfile() = scroll(CompilationMode.Partial(BaselineProfileMode.Require))

    private fun scroll(compilation: CompilationMode) {
        rule.measureRepeated(
            packageName = targetAppId,
            metrics = listOf(FrameTimingMetric()),
            iterations = ITERATIONS,
            // Not a startup mode: any of them ends the process between the setup and the
            // measurement, and there would be no list left to scroll.
            startupMode = null,
            compilationMode = compilation,
            setupBlock = {
                launchSignedIn()
                openLongList()
            },
        ) {
            flingList()
        }
    }

    private companion object {
        const val ITERATIONS: Int = 5
    }
}
