plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.ksp)
}

android {
    namespace = "ru.tinyops.turboist.core.database"
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
            // The replica is exercised against a real SQLite database on the JVM
            // rather than against stubs: a schema is only as correct as the
            // engine says it is. Resources have to be packaged for that to work.
            isIncludeAndroidResources = true
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

ksp {
    // The generated schema is checked in. It is the input a future migration
    // test reads the previous version from, and it makes an accidental schema
    // change visible in review as a diff rather than as a runtime crash.
    arg("room.schemaLocation", "$projectDir/schemas")
}

// The list-view contract fixture is shared with the server's own tests, so it
// lives outside this Gradle build. Handing the tests its location keeps them
// independent of whichever working directory the runner happens to pick.
tasks.withType<Test>().configureEach {
    val viewContractDir = rootProject.projectDir.parentFile.resolve("testdata/sync-contract")
    systemProperty("turboist.viewContract.dir", viewContractDir.absolutePath)
    // Declared as well as passed, for the same reason as the schema export
    // below: a fixture the tests read is an input to them, and without saying so
    // an edited fixture leaves the test task up to date, so the check reports
    // the previous run's verdict instead of running against the new data.
    inputs
        .dir(viewContractDir)
        .withPropertyName("viewContractFixture")
        .withPathSensitivity(PathSensitivity.RELATIVE)
    // The exported schema is an input to the tests as well as a build output:
    // one of them reads it back and holds it to what the engine actually built.
    // It has to be declared, not just passed — the directory is written by an
    // annotation-processor argument, so nothing else tells Gradle the tests
    // depend on it, and an up-to-date check that ignores it would skip the one
    // test whose whole job is to notice that the file changed.
    systemProperty("turboist.schemaExport.dir", "$projectDir/schemas")
    inputs
        .dir(layout.projectDirectory.dir("schemas"))
        .withPropertyName("roomSchemaExport")
        .withPathSensitivity(PathSensitivity.RELATIVE)
}

dependencies {
    // The replica speaks in domain types, so every consumer of this module gets them too.
    api(projects.core.model)

    api(libs.room.runtime)
    api(libs.room.ktx)
    ksp(libs.room.compiler)

    testImplementation(libs.kotlin.test.junit)
    testImplementation(libs.junit)
    testImplementation(libs.robolectric)
    testImplementation(libs.kotlinx.coroutines.test)
    // Reading the shared view-contract fixture and its goldens.
    testImplementation(libs.kotlinx.serialization.json)
}
