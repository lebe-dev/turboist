package turboist.i18n

/**
 * Turns the shared locale files into a complete set of Android resource files.
 *
 * The web and the native client show the same product, so they must show the
 * same words. Rather than copying wording into the Android project — where it
 * would drift the first time a phrase is reworded on one side only — the locale
 * files are the single source and these resources are derived from them on every
 * build.
 */
object LocaleStringGenerator {
    /** One locale file: an Android language code and the file's contents. */
    data class LocaleFile(val code: String, val json: String)

    private const val NOTE =
        "Generated from the shared locale files the web client is translated from.\n" +
            "Editing this file has no effect: it is rewritten on every build.\n" +
            "Change the wording in the locale file instead.\n" +
            "Strings that only the native client shows belong in strings_native.xml."

    /**
     * Builds `values/strings.xml` for [source] and `values-<code>/strings.xml`
     * for each translation, keyed by their path under a resource directory.
     *
     * A key the source has and a translation lacks is simply absent from that
     * translation's file; Android then falls back to the source resource, which
     * shows the English wording rather than a blank. Nothing is synthesised to
     * fill the gap. A key only a translation has is dropped: the source locale
     * decides which strings exist, and a translation-only key is wording no call
     * site can ever ask for.
     *
     * [handWrittenNames] are the resource names the checked-in resource files
     * already declare. A generated name that lands on one of them would let two
     * unrelated pieces of wording fight over the same identifier, with the winner
     * decided by resource-merge order, so the collision fails the build instead.
     */
    fun generate(
        source: LocaleFile,
        translations: List<LocaleFile>,
        handWrittenNames: Set<String>,
    ): Map<String, String> {
        val sourceMessages = LocaleMessages.flatten(source.json)
        val argumentOrders = sourceMessages.mapValues { (name, message) ->
            named(name) { AndroidStrings.argumentOrder(MessageParser.parse(message)) }
        }

        val collisions = sourceMessages.keys.intersect(handWrittenNames).sorted()
        require(collisions.isEmpty()) {
            "generated wording collides with hand-written resources: ${collisions.joinToString(", ")}. " +
                "Rename the hand-written resource, which by convention is prefixed 'native_'."
        }

        val files = LinkedHashMap<String, String>()
        files["values/strings.xml"] = renderLocale(sourceMessages, argumentOrders)
        for (translation in translations) {
            val messages = LocaleMessages.flatten(translation.json)
                .filterKeys { it in sourceMessages }
            files["values-${translation.code}/strings.xml"] = renderLocale(messages, argumentOrders)
        }
        return files
    }

    private fun renderLocale(
        messages: Map<String, String>,
        argumentOrders: Map<String, List<String>>,
    ): String {
        val resources = messages.map { (name, message) ->
            named(name) { AndroidStrings.toResource(name, message, argumentOrders[name].orEmpty()) }
        }
        return StringResourceXml.render(resources, NOTE)
    }

    /**
     * Names the offending locale key on failure. A converter error otherwise
     * describes a message with no way to find it among nearly a thousand.
     */
    private fun <T> named(name: String, block: () -> T): T =
        try {
            block()
        } catch (failure: IllegalArgumentException) {
            throw IllegalArgumentException("locale key '$name': ${failure.message}", failure)
        }
}
