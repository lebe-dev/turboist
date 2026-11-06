plugins {
    alias(libs.plugins.android.application) apply false
    alias(libs.plugins.android.library) apply false
    alias(libs.plugins.android.test) apply false
    alias(libs.plugins.baselineprofile) apply false
    alias(libs.plugins.kotlin.android) apply false
    alias(libs.plugins.kotlin.jvm) apply false
    alias(libs.plugins.kotlin.compose) apply false
    alias(libs.plugins.kotlin.serialization) apply false
    alias(libs.plugins.ksp) apply false
    alias(libs.plugins.hilt) apply false
    alias(libs.plugins.ktlint) apply false
    alias(libs.plugins.detekt) apply false
    alias(libs.plugins.sentry) apply false
}

subprojects {
    // Every module is formatted and checked by the same ktlint configuration;
    // `ktlintCheck` is what the lint entry point runs.
    apply(plugin = "org.jlleitschuh.gradle.ktlint")

    extensions.configure<org.jlleitschuh.gradle.ktlint.KtlintExtension> {
        // Generated sources (Room, Hilt, KSP) are not ours to format.
        filter {
            exclude { it.file.path.contains("/build/") }
        }
    }
    // ktlint owns formatting, Android Lint owns the platform; detekt looks for
    // code smells and likely bugs, and the Compose rules for misuse of the UI
    // toolkit. Findings that predate it are listed per module in a baseline, so
    // it fails on new code without first demanding a rewrite of the old.
    apply(plugin = "io.gitlab.arturbosch.detekt")

    extensions.configure<io.gitlab.arturbosch.detekt.extensions.DetektExtension> {
        buildUponDefaultConfig = true
        config.setFrom(rootProject.layout.projectDirectory.file("detekt.yml"))
        baseline = layout.projectDirectory.file("detekt-baseline.xml").asFile
        parallel = true
        // The default leaves the on-device suite out.
        source.from("src/androidTest/kotlin", "src/androidTest/java")
    }

    dependencies {
        add("detektPlugins", rootProject.libs.compose.rules.detekt)
    }
}
