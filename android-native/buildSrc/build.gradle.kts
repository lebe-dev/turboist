plugins {
    `kotlin-dsl`
}

repositories {
    mavenCentral()
}

// The shrinker audit reads things that live outside this project: the modules'
// sources, and the rules files that ship with them. Both are inputs to the
// check, and Gradle has no other way to know it — an edited source or an edited
// rule would otherwise leave the task up to date, so the build would report the
// previous run's verdict instead of running against the new state.
//
// The modules come from the build's own module list for the same reason the
// audit itself reads it: a list kept by hand here would quietly stop covering a
// module somebody added.
val moduleList = layout.projectDirectory.file("../settings.gradle.kts").asFile
val auditedModules =
    Regex("""include\(\s*"(:[A-Za-z0-9_\-:]+)"\s*\)""")
        .findAll(moduleList.readText())
        .map { it.groupValues[1].removePrefix(":").replace(':', '/') }
        .toList()

tasks.withType<Test>().configureEach {
    inputs
        .file(moduleList)
        .withPropertyName("moduleList")
        .withPathSensitivity(PathSensitivity.RELATIVE)
    auditedModules.forEach { module ->
        // The whole source tree of the module, not just one source set: a
        // flavour or build type added later has to re-run the audit too.
        inputs
            .dir(layout.projectDirectory.dir("../$module/src"))
            .withPropertyName("sources: $module")
            .withPathSensitivity(PathSensitivity.RELATIVE)
        inputs
            .files(
                layout.projectDirectory.file("../$module/consumer-rules.pro"),
                layout.projectDirectory.file("../$module/proguard-rules.pro"),
            ).withPropertyName("shrinkerRules: $module")
            .withPathSensitivity(PathSensitivity.RELATIVE)
    }
}

dependencies {
    testImplementation(kotlin("test"))
    testImplementation("junit:junit:4.13.2")
}
