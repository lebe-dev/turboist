package ru.tinyops.turboist.core.model.labelsuggest

/**
 * The exact text each thing is embedded as.
 *
 * The model was trained with task prefixes: a title is asked about as a
 * classification query, while a label or a project is given as a document with
 * a title and a body. The wording here is part of the calibration in
 * [LabelSuggestionRecipe] — a different prefix moves every score.
 */
object SuggestionTexts {
    private const val QUERY_PREFIX = "task: classification | query: "
    private const val PROJECTS_MARKER = "Projects: "

    /** A task title, as it is compared with everything else. */
    fun query(title: String): String = QUERY_PREFIX + clean(title)

    /**
     * A label: its name, its description, and the projects that carry it with
     * their descriptions — the projects say what a terse label is used for.
     */
    fun labelDocument(
        name: String,
        description: String,
        projects: List<ProjectText>,
    ): String {
        val parts = mutableListOf<String>()
        description.trim().takeIf { it.isNotEmpty() }?.let(parts::add)
        if (projects.isNotEmpty()) {
            parts += PROJECTS_MARKER + projects.joinToString("; ") { it.withDescription() }
        }
        return document(name, parts.joinToString(" "))
    }

    /** A project: its title and its description. */
    fun projectDocument(project: ProjectText): String = document(project.title, project.usefulDescription())

    private fun document(
        title: String,
        text: String,
    ): String {
        val heading = clean(title)
        val body = clean(text).ifEmpty { heading }
        return "title: $heading | text: $body"
    }

    private fun ProjectText.withDescription(): String {
        val description = usefulDescription()
        return if (description.isEmpty()) title else "$title ($description)"
    }

    /**
     * A description that is only a link says nothing the model can read, and a
     * long URL would crowd out the words that do.
     */
    private fun ProjectText.usefulDescription(): String {
        val trimmed = description.trim()
        return if (trimmed.startsWith("http")) "" else trimmed
    }

    private val whitespace = Regex("\\s+")

    private fun clean(text: String): String = text.trim().replace(whitespace, " ")
}

/** A project's two texts, which is all a suggestion reads of it. */
data class ProjectText(
    val title: String,
    val description: String = "",
)
