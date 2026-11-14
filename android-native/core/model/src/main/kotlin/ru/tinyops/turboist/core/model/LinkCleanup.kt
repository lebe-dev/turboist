package ru.tinyops.turboist.core.model

/**
 * Removes advertising and analytics parameters (`utm_*`, `gclid`, `fbclid`, …)
 * from the links in free text, so a link pasted from a browser or a newsletter
 * is saved the way it would be shared by hand. The web client does the same on
 * its side; the two lists are kept in step.
 */
object LinkCleanup {
    private val linkPattern = Regex("https?://[^\\s<>\"]+", RegexOption.IGNORE_CASE)

    /** What may follow a link in prose or markdown without being part of it. */
    private val trailingPunctuation = Regex("[.,;:!?)\\]}']+$")

    private val trackingParams =
        setOf(
            "gclid", "gclsrc", "dclid", "gbraid", "wbraid", "fbclid", "msclkid", "yclid", "ysclid",
            "mc_cid", "mc_eid", "igshid", "_ga", "_gl",
        )

    fun strip(text: String): String {
        if (!text.contains("://")) return text
        return linkPattern.replace(text) { match ->
            val raw = match.value
            val trailing = trailingPunctuation.find(raw)?.value.orEmpty()
            cleanLink(raw.dropLast(trailing.length)) + trailing
        }
    }

    /**
     * The text a field should hold after an edit from [before] to [after]: a
     * paste or a share (more than one character arriving at once) is cleaned,
     * typing is left alone so the caret never jumps under the person's hands.
     */
    fun afterEdit(
        before: String,
        after: String,
    ): String = if (after.length - before.length > 1) strip(after) else after

    private fun cleanLink(link: String): String {
        val hashAt = link.indexOf('#')
        val fragment = if (hashAt >= 0) link.substring(hashAt) else ""
        val rest = if (hashAt >= 0) link.substring(0, hashAt) else link
        val queryAt = rest.indexOf('?')
        if (queryAt < 0) return link
        val kept = rest.substring(queryAt + 1).split('&').filter { it.isNotEmpty() && !isTracking(it) }
        val base = rest.substring(0, queryAt)
        return (if (kept.isEmpty()) base else base + "?" + kept.joinToString("&")) + fragment
    }

    private fun isTracking(pair: String): Boolean {
        val name = pair.substringBefore('=').lowercase()
        return name.startsWith("utm_") || name in trackingParams
    }
}
