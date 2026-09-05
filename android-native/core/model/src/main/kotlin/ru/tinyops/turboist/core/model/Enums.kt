package ru.tinyops.turboist.core.model

/**
 * How urgent a task is. The lowest level is spelled `no-priority` on the wire —
 * it is a real value, not an absent one.
 */
enum class Priority(override val wire: String) : WireEnum {
    HIGH("high"),
    MEDIUM("medium"),
    LOW("low"),
    NONE("no-priority"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<Priority>(entries, UNKNOWN)
}

/**
 * Where a task stands. Both [COMPLETED] and [CANCELLED] count as closed: either
 * one releases the tasks this one was blocking.
 */
enum class TaskStatus(override val wire: String) : WireEnum {
    OPEN("open"),
    COMPLETED("completed"),
    CANCELLED("cancelled"),
    UNKNOWN(""),
    ;

    /** True when the task no longer holds anything up and no longer appears in a list of work. */
    val isClosed: Boolean
        get() = this == COMPLETED || this == CANCELLED

    companion object : WireEnumLookup<TaskStatus>(entries, UNKNOWN)
}

/** Where a project stands. Archived projects stay readable but are out of the way. */
enum class ProjectStatus(override val wire: String) : WireEnum {
    OPEN("open"),
    COMPLETED("completed"),
    ARCHIVED("archived"),
    CANCELLED("cancelled"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<ProjectStatus>(entries, UNKNOWN)
}

/**
 * What kind of board a project renders as. The server substitutes [GENERIC] for
 * an empty value, so a project always arrives with one of the known types.
 */
enum class ProjectType(override val wire: String) : WireEnum {
    GENERIC("generic"),
    SOFTWARE("software"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<ProjectType>(entries, UNKNOWN)
}

/**
 * Whether the user has committed a task to the current week or parked it.
 * Parking a task cascades to its open subtasks, and so does planning it — the
 * cascade is the server's, this type only names the states.
 */
enum class PlanState(override val wire: String) : WireEnum {
    NONE("none"),
    WEEK("week"),
    BACKLOG("backlog"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<PlanState>(entries, UNKNOWN)
}

/**
 * Which phase of the day a task is meant for. [NONE] means "any time today" and
 * is a real value; it is not the unknown sentinel.
 */
enum class DayPart(override val wire: String) : WireEnum {
    NONE("none"),
    MORNING("morning"),
    AFTERNOON("afternoon"),
    EVENING("evening"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<DayPart>(entries, UNKNOWN)
}

/** The three capacity buckets of the daily plan. */
enum class TroikiCategory(override val wire: String) : WireEnum {
    IMPORTANT("important"),
    MEDIUM("medium"),
    REST("rest"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<TroikiCategory>(entries, UNKNOWN)
}

/**
 * The kind of link between two tasks. [RELATED] is symmetric and purely
 * informational; [BLOCKS] is directed and enforced — a task cannot be completed
 * while any task blocking it is still open.
 */
enum class RelationType(override val wire: String) : WireEnum {
    RELATED("related"),
    BLOCKS("blocks"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<RelationType>(entries, UNKNOWN)
}

/**
 * A `blocks` relation seen from one of its two endpoints: [OUTGOING] means this
 * task blocks the peer, [INCOMING] means the peer blocks this task. Meaningless
 * for [RelationType.RELATED], which is symmetric.
 */
enum class RelationDirection(override val wire: String) : WireEnum {
    OUTGOING("outgoing"),
    INCOMING("incoming"),
    UNKNOWN(""),
    ;

    /** The same edge as the peer at the other end reads it. */
    val opposite: RelationDirection
        get() =
            when (this) {
                OUTGOING -> INCOMING
                INCOMING -> OUTGOING
                UNKNOWN -> UNKNOWN
            }

    companion object : WireEnumLookup<RelationDirection>(entries, UNKNOWN)
}

/**
 * Which client a session belongs to. Sessions are capped per kind, and this
 * native client shares the [ANDROID] budget with the WebView shell.
 */
enum class ClientKind(override val wire: String) : WireEnum {
    WEB("web"),
    IOS("ios"),
    CLI("cli"),
    ANDROID("android"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<ClientKind>(entries, UNKNOWN)
}

/** What a harpoon slot points at. */
enum class HarpoonKind(override val wire: String) : WireEnum {
    TASK("task"),
    PROJECT("project"),
    UNKNOWN(""),
    ;

    companion object : WireEnumLookup<HarpoonKind>(entries, UNKNOWN)
}
