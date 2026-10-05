plugins {
    alias(libs.plugins.kotlin.jvm)
}

// Deliberately a plain Kotlin/JVM library with no production dependencies at
// all: the domain types must stay free of Android APIs, of persistence
// annotations and of serialization annotations, so they can be exercised by
// fast JVM tests and so the layers above map onto them instead of the other way
// round.

java {
    sourceCompatibility = JavaVersion.VERSION_17
    targetCompatibility = JavaVersion.VERSION_17
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    testImplementation(libs.kotlin.test.junit)
    testImplementation(libs.junit)
}
