package ru.tinyops.turboist.core.database.search

/**
 * Turns what a person typed into a query the full-text index understands.
 *
 * It sits here, beside the index, rather than in a screen: the syntax it emits
 * is a property of the index — how a column is addressed, what a trailing star
 * means — and a second dialect written somewhere else would be a second thing to
 * keep true. What a screen decides is *when* to search; what a term means is
 * decided once, here.
 *
 * Three rules, and they are the whole of it:
 *
 * - **Only letters and digits survive.** Everything else is a separator. That is
 *   what makes the builder total: the index's own query language uses quotes,
 *   stars, colons, minus signs and parentheses as operators, and a person typing
 *   `c++ "urgent"` means none of them. Splitting on non-alphanumerics leaves
 *   terms that cannot contain an operator, so nothing has to be escaped and no
 *   input can be refused as malformed.
 * - **Every term is a prefix.** Search happens while the user is still typing,
 *   so `mol` has to find `molecule` — an exact-match search would show nothing
 *   until the last keystroke and look broken.
 * - **Terms are combined with AND.** Two words narrow; a row has to carry both.
 *   That is what the index does with a space between terms, and it is what a
 *   person means by typing a second word.
 *
 * Matching itself is the tokenizer's business, not this builder's: it folds case
 * and diacritics across scripts, so `cafe` finds `Café` and `МОЛОКО` finds
 * `молоко` without a single character being touched here.
 */
object FtsQuery {
    /**
     * The shortest query worth running, in characters the user typed.
     *
     * The same floor the web client applies. A single letter matches a large
     * share of a real workspace through the prefix rule, so answering it is both
     * slow and useless — the screen keeps its resting state instead.
     */
    const val MINIMUM_LENGTH: Int = 2

    /** The column a title-weighted match is scoped to in the task and project indexes. */
    const val TITLE_COLUMN: String = "title"

    private val SEPARATORS = Regex("[^\\p{L}\\p{N}]+")

    /** The words in what was typed, with everything that is not one dropped. */
    fun terms(typed: String): List<String> = typed.split(SEPARATORS).filter { it.isNotEmpty() }

    /**
     * A query over every indexed column, or `null` when there is nothing to ask.
     *
     * `null` means "do not search" rather than "match nothing": the two are
     * different screens, and a query of two spaces has to leave the resting state
     * on screen rather than report that the workspace holds nothing.
     */
    fun match(typed: String): String? = build(typed, column = null)

    /**
     * The same terms, restricted to one column.
     *
     * Used to tell a title hit from a body hit so the ranking can prefer the
     * first. It is a second question about the same terms, not a second search.
     */
    fun matchIn(
        column: String,
        typed: String,
    ): String? = build(typed, column)

    private fun build(
        typed: String,
        column: String?,
    ): String? {
        if (typed.trim().length < MINIMUM_LENGTH) return null
        val terms = terms(typed)
        if (terms.isEmpty()) return null
        val scope = if (column == null) "" else "$column:"
        return terms.joinToString(" ") { "$scope$it*" }
    }
}
