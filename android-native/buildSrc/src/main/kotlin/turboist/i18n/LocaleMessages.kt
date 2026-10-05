package turboist.i18n

/**
 * Reads the shared locale files the web client is translated from and turns
 * their nested objects into the flat `a_b_c` names Android resources use.
 *
 * The wording lives in exactly one place for both clients, so nothing here may
 * invent, translate or drop a message: a locale file is read as-is and only its
 * key shape changes.
 */
object LocaleMessages {
    /** Android accepts a resource name that reads as a Java identifier and nothing else. */
    private val RESOURCE_NAME = Regex("""[A-Za-z][A-Za-z0-9_]*""")

    /**
     * Flattens one locale file into `resource name -> message`, ordered by
     * resource name so the generated XML is byte-identical across builds.
     *
     * Nested objects contribute their path, so `a.b.c` becomes `a_b_c`. Because
     * a key may itself contain an underscore, two different paths can flatten
     * onto the same name; that is silent breakage at runtime, so it fails here
     * instead.
     */
    fun flatten(source: String): Map<String, String> {
        val flat = LinkedHashMap<String, String>()
        val origin = LinkedHashMap<String, String>()
        collect(JsonTree.parse(source), path = "", flat = flat, origin = origin)
        return flat.toSortedMap()
    }

    private fun collect(
        node: JsonNode.Group,
        path: String,
        flat: MutableMap<String, String>,
        origin: MutableMap<String, String>,
    ) {
        for ((key, value) in node.entries) {
            val dotted = if (path.isEmpty()) key else "$path.$key"
            when (value) {
                is JsonNode.Group -> collect(value, dotted, flat, origin)
                is JsonNode.Message -> {
                    val name = resourceName(dotted)
                    val previous = origin[name]
                    require(previous == null) {
                        "locale keys '$previous' and '$dotted' both flatten to the resource name '$name'"
                    }
                    origin[name] = dotted
                    flat[name] = value.text
                }
            }
        }
    }

    /** Turns a dotted locale key into an Android resource name. */
    fun resourceName(dotted: String): String {
        val name = dotted.replace('.', '_')
        require(RESOURCE_NAME.matches(name)) {
            "locale key '$dotted' does not map onto a legal Android resource name"
        }
        return name
    }
}
