package ru.tinyops.turboist.nativeapp.settings

/**
 * Where the two legal documents live.
 *
 * They are served by the installation the user connected to rather than shipped
 * inside the app, and that is the whole point: the product is self-hosted, so
 * the terms that apply are the ones the operator of *that* server publishes.
 * Copying them into the package would show a document nobody agreed to, and one
 * that goes stale the moment the server's is edited.
 *
 * They are the same two paths the web client links to, so the phone and the
 * browser show the same text.
 */
object LegalDocuments {
    const val TERMS_PATH: String = "terms-of-service"
    const val PRIVACY_PATH: String = "privacy-policy"

    /**
     * The address of one document on the connected server, or an empty string
     * when there is no server yet — a link with nowhere to go is not offered.
     *
     * The stored address always ends in a slash, so a path appends to it instead
     * of replacing its last segment; an address that somehow does not is given
     * one here rather than producing a URL that quietly drops a path prefix.
     */
    fun urlFor(
        serverAddress: String,
        path: String,
    ): String {
        val trimmed = serverAddress.trim()
        if (trimmed.isEmpty()) return ""
        return if (trimmed.endsWith("/")) trimmed + path else "$trimmed/$path"
    }
}
