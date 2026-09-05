plugins {
    alias(libs.plugins.android.library)
    alias(libs.plugins.kotlin.android)
    alias(libs.plugins.kotlin.serialization)
}

android {
    namespace = "ru.tinyops.turboist.core.network"
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
            // The HTTP layer is plain JVM code exercised against a local mock
            // server. Returning defaults instead of throwing keeps an incidental
            // framework call from failing a test that is not about the framework.
            isReturnDefaultValues = true
        }
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    // Wire DTOs map onto domain types, so every consumer of this module gets them too.
    api(projects.core.model)

    // Retrofit, OkHttp and the JSON format are part of this module's surface:
    // callers hold the generated endpoint interfaces and catch failures that
    // extend IOException, so they must see those types.
    api(libs.retrofit)
    api(libs.okhttp)
    api(libs.kotlinx.serialization.json)
    api(libs.kotlinx.coroutines.core)
    implementation(libs.retrofit.serialization)
    // The change stream is read as it arrives, which the generated endpoints cannot do.
    api(libs.okhttp.sse)
    implementation(libs.okhttp.logging)

    testImplementation(libs.kotlin.test.junit)
    testImplementation(libs.junit)
    testImplementation(libs.kotlinx.coroutines.test)
    testImplementation(libs.okhttp.mockwebserver)
}
