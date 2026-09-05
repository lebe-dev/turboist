import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import turboist.AppVersion
import turboist.ReleaseSigning
import turboist.buildVersionName
import turboist.i18n.GenerateLocaleStringsTask

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.compose)
    alias(libs.plugins.kotlin.serialization)
    alias(libs.plugins.ksp)
    alias(libs.plugins.hilt)
}

// One version for the whole repository: the app reads the same file the backend
// binary and the web bundle are stamped from, so a build can never claim a
// version the server does not know about.
val versionNameFromRepo: String =
    providers.fileContents(rootProject.layout.projectDirectory.file("../VERSION"))
        .asText
        .map(AppVersion::normalize)
        .get()

// A build somebody is handed also carries the commit it was made from, so a
// report about it identifies the exact code that produced it. It is a build
// identifier and never affects release ordering, which is why the numeric code
// below is derived from the bare version instead.
val buildStamp: String? = providers.gradleProperty("turboist.buildStamp").orNull

// A release artifact is signed with the upload key when the machine building it
// supplies one, and left unsigned when it does not, so `assembleRelease` still
// runs on a machine that holds no secrets. The four values are read from Gradle
// properties or from the environment; none of them is ever written into the
// repository, and supplying only some of them stops the build rather than
// producing an artifact that cannot be uploaded.
val releaseSigning =
    ReleaseSigning.from { name ->
        providers.gradleProperty(name).orNull ?: providers.environmentVariable(name).orNull
    }

android {
    namespace = "ru.tinyops.turboist.nativeapp"
    compileSdk = 36

    defaultConfig {
        // Distinct from the WebView shell's id so both can be installed side by side.
        applicationId = "ru.tinyops.turboist.native"
        minSdk = 28
        targetSdk = 36
        versionCode = AppVersion.versionCode(versionNameFromRepo)
        versionName = AppVersion.buildVersionName(versionNameFromRepo, buildStamp)
        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"
    }

    signingConfigs {
        releaseSigning?.let { credentials ->
            create("release") {
                // A relative path is read against the repository root, so a build
                // machine can name the key it dropped beside the checkout without
                // depending on which directory Gradle was started from.
                storeFile = rootProject.file(credentials.storeFile)
                storePassword = credentials.storePassword
                keyAlias = credentials.keyAlias
                keyPassword = credentials.keyPassword
            }
        }
    }

    buildTypes {
        debug {
            isMinifyEnabled = false
        }
        release {
            // What ships is minified. Beyond the size, it is the only build that
            // exercises the shrinker rules, and a rule that is missing shows up
            // as a crash rather than as a broken build — so the release build has
            // to be the minified one everywhere, not only on the machine that
            // eventually uploads it.
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            // Absent on a machine that supplies no key: the artifact is then
            // built unsigned rather than the build failing.
            signingConfig = signingConfigs.findByName("release")
        }
    }

    buildFeatures {
        compose = true
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    lint {
        abortOnError = true
        warningsAsErrors = false
        // A phrase translated in English but not yet in Russian is not an error:
        // Android falls back to the English resource, which is the same fallback
        // the web client applies, and shows real wording instead of a blank.
        disable += "MissingTranslation"
    }

    testOptions {
        unitTests {
            // Unit tests exercise plain Kotlin (routes, palettes, counters).
            // Returning defaults instead of throwing keeps an incidental
            // framework call from failing a test that is not about the framework.
            isReturnDefaultValues = true
            // The navigation walk composes the real shell on the JVM, so it needs
            // the packaged resources: the drawer entries and screen titles it
            // clicks and reads are string resources, and a walk against blank
            // labels would prove nothing.
            isIncludeAndroidResources = true
            // The screen tests compose real Compose trees against real resources,
            // and every one of them in this module runs in a single worker JVM.
            // The default heap for that worker is small enough that the suite
            // runs out of it partway through, which surfaces as a wall of
            // unrelated screens failing at once rather than as anything to do
            // with the screen being tested.
            all { test -> test.maxHeapSize = "4g" }
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_17)
    }
}

// What the drawing layer may take for granted about the types it is handed.
// Stated in a file of its own so the claim sits next to the reasoning for it.
composeCompiler {
    stabilityConfigurationFiles.add(rootProject.layout.projectDirectory.file("compose-stability.conf"))
    // Which composables the compiler was able to make skippable, and why each
    // parameter counted as it did. Off by default because it is an answer nobody
    // needs on an ordinary build; run with `-Pturboist.composeReports` when the
    // question is why a screen redraws more than it should.
    if (providers.gradleProperty("turboist.composeReports").isPresent) {
        reportsDestination.set(layout.buildDirectory.dir("compose-reports"))
    }
}

// Product wording is written once, in the locale files the web client is
// translated from, and projected onto Android resources on every build. Editing
// the generated files is pointless; change the locale file instead.
androidComponents {
    onVariants { variant ->
        val taskName = "generate${variant.name.replaceFirstChar { it.uppercase() }}LocaleStrings"
        val handWritten = project.fileTree("src/main/res") { include("values*/*.xml") }
        val generate =
            tasks.register<GenerateLocaleStringsTask>(taskName) {
                description = "Generates string resources from the shared locale files."
                localesDirectory.set(rootProject.layout.projectDirectory.dir("../frontend/locales"))
                sourceLocale.set("en")
                translationLocales.set(listOf("ru"))
                // Checked against the hand-written resources so the two sets can
                // never claim the same name and let merge order pick the wording.
                handWrittenResources.from(handWritten)
            }
        variant.sources.res?.addGeneratedSourceDirectory(
            generate,
            GenerateLocaleStringsTask::outputDirectory,
        )
    }
}

dependencies {
    implementation(projects.core.model)
    implementation(projects.core.database)
    implementation(projects.core.network)
    implementation(projects.core.sync)

    implementation(platform(libs.compose.bom))
    implementation(libs.compose.material3)
    implementation(libs.compose.ui)
    implementation(libs.compose.ui.graphics)
    implementation(libs.compose.ui.tooling.preview)
    implementation(libs.compose.material.icons.extended)
    debugImplementation(libs.compose.ui.tooling)

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.lifecycle.viewmodel.compose)
    implementation(libs.androidx.lifecycle.runtime.compose)
    // Whether the app is in front of the user is a property of the process,
    // not of whichever activity happens to exist: the change stream must
    // survive a screen rotation and stop when the app is put away.
    implementation(libs.androidx.lifecycle.process)
    implementation(libs.androidx.navigation.compose)
    implementation(libs.kotlinx.serialization.json)
    // The session layer's two small persisted values: the server address, and the
    // refresh token sealed by a key that never leaves the platform keystore.
    implementation(libs.datastore.preferences)

    // Passkeys go through Credential Manager: the platform picks the
    // authenticator and draws the sheet, and the Play services provider is what
    // backs it on a device with Google Password Manager.
    implementation(libs.androidx.credentials)
    implementation(libs.androidx.credentials.play.services)

    implementation(libs.hilt.android)
    implementation(libs.hilt.navigation.compose)
    ksp(libs.hilt.compiler)

    testImplementation(libs.kotlin.test.junit)
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    // The shell's navigation is proved by composing it on the JVM rather than on
    // a device: the drawer is the only way into most of the app, and a walk
    // through it is the one check that the destinations, the graph and the
    // titles agree. Robolectric hosts the Android framework the composition
    // needs; the test manifest supplies the activity the harness launches into.
    // Both are tied to the debug variant, because the activity comes from a
    // manifest that must never be merged into a build that ships.
    debugImplementation(libs.compose.ui.test.manifest)
    testDebugImplementation(platform(libs.compose.bom))
    testDebugImplementation(libs.compose.ui.test.junit4)
    testDebugImplementation(libs.robolectric)

    // The on-device suite runs inside the installed app and drives its real
    // object graph against a real server, so it compiles against every module
    // the app is built from. It also speaks HTTP itself: the checks it makes are
    // about what the server ended up holding, which only the server can answer.
    androidTestImplementation(projects.core.model)
    androidTestImplementation(projects.core.database)
    androidTestImplementation(projects.core.network)
    androidTestImplementation(projects.core.sync)
    androidTestImplementation(libs.junit)
    androidTestImplementation(libs.androidx.test.runner)
    androidTestImplementation(libs.androidx.test.core)
    androidTestImplementation(libs.androidx.test.ext.junit)
    androidTestImplementation(libs.kotlinx.coroutines.core)
    androidTestImplementation(libs.kotlinx.serialization.json)
    androidTestImplementation(libs.okhttp)
    androidTestImplementation(libs.hilt.android)
}
