package ru.tinyops.turboist.nativeapp.account

/**
 * What an API token may be allowed to touch.
 *
 * The names are the server's, because they are what travels: a scope is
 * `<resource>:<action>`, and a resource this build spelled differently would be
 * refused outright rather than quietly narrowed.
 *
 * @property writable whether the resource has a write half at all. Search and
 *   the calendar are read-only surfaces — there is no write scope to grant, and
 *   offering an unselectable checkbox for one would be a lie about what a token
 *   can do.
 */
enum class ScopeResource(
    val wire: String,
    val writable: Boolean = true,
) {
    TASKS("tasks"),
    PROJECTS("projects"),
    CONTEXTS("contexts"),
    LABELS("labels"),
    SECTIONS("sections"),
    TEMPLATES("templates"),
    TROIKI("troiki"),
    SETTINGS("settings"),
    SEARCH("search", writable = false),
    CALENDARS("calendars", writable = false),
    ;

    val read: String get() = "$wire:$READ"

    val write: String get() = "$wire:$WRITE"

    companion object {
        const val READ = "read"
        const val WRITE = "write"

        /** The scope standing for everything, now and in future versions of the API. */
        const val WILDCARD = "*"
    }
}

/**
 * The permissions being granted to a token that has not been created yet.
 *
 * Two rules from the server are enforced here rather than discovered on a
 * rejection, because both are about what the user is allowed to *ask for* and
 * both are cheaper to answer while the form is still on screen:
 *
 * - write does not imply read, so a token granted write alone could change work
 *   it cannot see. The server refuses that outright, so ticking write ticks read
 *   with it and unticking read takes write away.
 * - the wildcard may only stand alone. It is not "every box ticked" — it also
 *   covers scopes a future server adds — so choosing it clears the boxes, and
 *   touching a box drops it.
 *
 * Scopes are immutable once a token exists; there is no endpoint that widens or
 * narrows one. What this produces is therefore the whole of what the token will
 * ever be able to do.
 */
data class ScopeSelection(
    val fullAccess: Boolean = false,
    val read: Set<ScopeResource> = emptySet(),
    val write: Set<ScopeResource> = emptySet(),
) {
    /** True when nothing is granted, which is a token the server would refuse. */
    val isEmpty: Boolean get() = !fullAccess && read.isEmpty() && write.isEmpty()

    /** Grants or withdraws the whole of the API, clearing any resource-by-resource choice. */
    fun withFullAccess(granted: Boolean): ScopeSelection = if (granted) FULL_ACCESS else copy(fullAccess = false)

    /**
     * Grants or withdraws reading one resource. Withdrawing it withdraws writing
     * too: a token that may change what it cannot read is refused by the server.
     */
    fun withRead(
        resource: ScopeResource,
        granted: Boolean,
    ): ScopeSelection =
        copy(
            fullAccess = false,
            read = read.toggled(resource, granted),
            write = if (granted) write else write - resource,
        )

    /**
     * Grants or withdraws writing one resource. Granting it grants reading as
     * well, for the same reason. A resource with no write half never changes.
     */
    fun withWrite(
        resource: ScopeResource,
        granted: Boolean,
    ): ScopeSelection {
        if (!resource.writable) return this
        return copy(
            fullAccess = false,
            read = if (granted) read + resource else read,
            write = write.toggled(resource, granted),
        )
    }

    /**
     * The scopes as the server takes them: sorted by resource so two identical
     * selections always produce the same request, and the wildcard alone when it
     * is what was chosen.
     */
    fun scopes(): List<String> {
        if (fullAccess) return listOf(ScopeResource.WILDCARD)
        return ScopeResource.entries.flatMap { resource ->
            buildList {
                if (resource in read) add(resource.read)
                if (resource.writable && resource in write) add(resource.write)
            }
        }
    }

    private fun Set<ScopeResource>.toggled(
        resource: ScopeResource,
        granted: Boolean,
    ): Set<ScopeResource> = if (granted) this + resource else this - resource

    companion object {
        /** Nothing granted. The starting point of the form, and not a valid token. */
        val NONE = ScopeSelection()

        /** The whole API, including scopes a later server version introduces. */
        val FULL_ACCESS = ScopeSelection(fullAccess = true)

        /** Every resource readable, none writable — the shape most integrations need. */
        val READ_ONLY = ScopeSelection(read = ScopeResource.entries.toSet())

        /** Read and write on the work itself, and nothing else. */
        val TASKS_FULL = ScopeSelection(read = setOf(ScopeResource.TASKS), write = setOf(ScopeResource.TASKS))
    }
}

/**
 * True when a token was granted everything.
 *
 * Tokens issued before scopes existed come back carrying the wildcard, so this
 * is also what tells the list they are unrestricted rather than unscoped.
 */
fun List<String>.grantsFullAccess(): Boolean = contains(ScopeResource.WILDCARD)
