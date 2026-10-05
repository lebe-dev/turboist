plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "ru.tinyops.turboist.core.sync"
    compileSdk = 36

    defaultConfig {
        minSdk = 28
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
        consumerProguardFiles("consumer-rules.pro")
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    lint {
        abortOnError = true
        warningsAsErrors = false
    }

    testOptions {
        unitTests {
            // The write path is exercised against a real SQLite database rather
            // than against stubs: "the optimistic row and the queued op commit
            // together" is a claim only a transaction engine can settle.
            isIncludeAndroidResources = true
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

// The board-ordering cases are shared with the server's own tests, so they live
// outside this Gradle build. Handing the tests their location keeps them
// independent of whichever working directory the runner happens to pick.
tasks.withType<Test>().configureEach {
    val syncContractDir = rootProject.projectDir.parentFile.resolve("testdata/sync-contract")
    systemProperty("turboist.syncContract.dir", syncContractDir.absolutePath)
    // Declared as well as passed: a fixture the tests read is an input to them,
    // and without saying so an edited fixture leaves the test task up to date,
    // so the check reports the previous run's verdict instead of running against
    // the new cases.
    inputs
        .dir(syncContractDir)
        .withPropertyName("syncContractFixture")
        .withPathSensitivity(PathSensitivity.RELATIVE)
}

// The worked examples the recurrence advance is checked against are shared with
// the server's own tests, so they live outside this Gradle build. Handing the
// tests their location keeps them independent of whichever working directory the
// runner picks, and declaring them as an input is what makes an edited example
// re-run the check instead of reporting the previous run's verdict.
tasks.withType<Test>().configureEach {
    val recurrenceContractDir = rootProject.projectDir.parentFile.resolve("testdata/recurrence-contract")
    systemProperty("turboist.recurrenceContract.dir", recurrenceContractDir.absolutePath)
    inputs
        .dir(recurrenceContractDir)
        .withPropertyName("recurrenceContract")
        .withPathSensitivity(PathSensitivity.RELATIVE)
}

dependencies {
    // The sync engine is the only place where the replica and the wire meet.
    api(projects.core.database)
    api(projects.core.network)

    // Syncing has to keep happening when the app is not running, which only the
    // platform's job scheduler can arrange. The app module wires the worker into
    // the dependency graph, so it sees these types too.
    api(libs.work.runtime.ktx)

    // Working out when a repeating task next falls due. Kept as an implementation
    // detail: the write path talks to its own seam, so the calendar library never
    // appears in a signature and can be swapped without touching a caller.
    implementation(libs.lib.recur)

    testImplementation(libs.kotlin.test.junit)
    testImplementation(libs.junit)
    testImplementation(libs.robolectric)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
    testImplementation(libs.work.testing)
    // Reading the shared board-ordering cases.
    testImplementation(libs.kotlinx.serialization.json)
}
