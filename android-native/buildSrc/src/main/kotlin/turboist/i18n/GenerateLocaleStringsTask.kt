package turboist.i18n

import org.gradle.api.DefaultTask
import org.gradle.api.file.ConfigurableFileCollection
import org.gradle.api.file.DirectoryProperty
import org.gradle.api.provider.ListProperty
import org.gradle.api.provider.Property
import org.gradle.api.tasks.CacheableTask
import org.gradle.api.tasks.Input
import org.gradle.api.tasks.InputDirectory
import org.gradle.api.tasks.InputFiles
import org.gradle.api.tasks.OutputDirectory
import org.gradle.api.tasks.PathSensitive
import org.gradle.api.tasks.PathSensitivity
import org.gradle.api.tasks.TaskAction
import java.io.File

/**
 * Generates the app's string resources from the shared locale files.
 *
 * Product wording is written once, in the locale files both clients read, and
 * this task projects it onto the Android resource system on every build. Editing
 * the generated files is pointless — they are overwritten — and the check at the
 * end of [generate] refuses a build where generated wording would shadow a
 * hand-written resource.
 */
@CacheableTask
abstract class GenerateLocaleStringsTask : DefaultTask() {
    /** Directory holding one `<code>.json` file per locale. */
    @get:InputDirectory
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val localesDirectory: DirectoryProperty

    /** The locale every other one falls back to; its file becomes `values/`. */
    @get:Input
    abstract val sourceLocale: Property<String>

    /** Locale codes to translate into, each rendered into `values-<code>/`. */
    @get:Input
    abstract val translationLocales: ListProperty<String>

    /** Checked-in resource files whose names generated wording must not shadow. */
    @get:InputFiles
    @get:PathSensitive(PathSensitivity.RELATIVE)
    abstract val handWrittenResources: ConfigurableFileCollection

    /** Resource directory the generated `strings.xml` files are written into. */
    @get:OutputDirectory
    abstract val outputDirectory: DirectoryProperty

    @TaskAction
    fun generate() {
        val locales = localesDirectory.get().asFile
        val source = sourceLocale.get()
        val handWritten = handWrittenResources.files
            .filter { it.isFile }
            .flatMap { StringResourceXml.declaredNames(it.readText()) }
            .toSet()

        val files = LocaleStringGenerator.generate(
            source = LocaleStringGenerator.LocaleFile(source, localeText(locales, source)),
            translations = translationLocales.get()
                .filter { it != source }
                .map { LocaleStringGenerator.LocaleFile(it, localeText(locales, it)) },
            handWrittenNames = handWritten,
        )

        val target = outputDirectory.get().asFile
        target.deleteRecursively()
        for ((path, content) in files) {
            val file = target.resolve(path)
            file.parentFile.mkdirs()
            file.writeText(content)
        }
        logger.info("Generated Android string resources for {} locales", files.size)
    }

    private fun localeText(directory: File, code: String): String {
        val file = directory.resolve("$code.json")
        require(file.isFile) { "locale file not found: ${file.absolutePath}" }
        return file.readText()
    }
}
