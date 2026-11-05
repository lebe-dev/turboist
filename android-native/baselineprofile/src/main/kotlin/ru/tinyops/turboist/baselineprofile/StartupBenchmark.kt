package ru.tinyops.turboist.baselineprofile

import androidx.benchmark.macro.BaselineProfileMode
import androidx.benchmark.macro.CompilationMode
import androidx.benchmark.macro.ExperimentalMetricApi
import androidx.benchmark.macro.MemoryUsageMetric
import androidx.benchmark.macro.StartupMode
import androidx.benchmark.macro.StartupTimingMetric
import androidx.benchmark.macro.junit4.MacrobenchmarkRule
import androidx.test.ext.junit.runners.AndroidJUnit4
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * A cold start onto the signed-in Today list, with the shipped profile and without.
 *
 * The first is what a person gets. The second is there because the difference
 * between them is the only evidence that the committed profile still does anything.
 */
@OptIn(ExperimentalMetricApi::class)
@RunWith(AndroidJUnit4::class)
class StartupBenchmark {
    @get:Rule
    val rule = MacrobenchmarkRule()

    @Test
    fun withoutProfile() = coldStart(CompilationMode.None())

    @Test
    fun withShippedProfile() = coldStart(CompilationMode.Partial(BaselineProfileMode.Require))

    private fun coldStart(compilation: CompilationMode) {
        var prepared = false
        rule.measureRepeated(
            packageName = targetAppId,
            metrics = listOf(StartupTimingMetric(), MemoryUsageMetric(MemoryUsageMetric.Mode.Last)),
            iterations = ITERATIONS,
            startupMode = StartupMode.COLD,
            compilationMode = compilation,
            setupBlock = {
                if (!prepared) {
                    launchSignedIn()
                    killProcess()
                    prepared = true
                }
                pressHome()
            },
        ) {
            startActivityAndWait()
        }
    }

    private companion object {
        const val ITERATIONS: Int = 10
    }
}
