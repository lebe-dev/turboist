dependencyResolutionManagement {
    repositories {
        mavenCentral()
    }
}

// Named so this build can also be run on its own (`gradlew -p buildSrc test`),
// which is how the version-derivation tests are executed.
rootProject.name = "build-logic"
