package ru.tinyops.turboist.core.database.sync

/**
 * Which replica table a bookkeeping row points at.
 *
 * The stored spellings are the server's own names for the same records, and they
 * are a contract in both directions: an incoming change names its entity with
 * one of these strings, and an outbox op names the row it will send with the
 * same one. Renaming a constant's [stored] value orphans every bookkeeping row
 * a device has already written, so the set is append-only.
 *
 * There is no member for the task↔label edge: the server does not report one
 * either. A label being attached or detached is reported as a change to the task
 * that carries it, because that is the record the payload describes.
 */
enum class ReplicaEntityKind(val stored: String) {
    TASK("task"),
    PROJECT("project"),
    SECTION("section"),
    CONTEXT("context"),
    LABEL("label"),
    TASK_RELATION("task_relation"),
    TASK_TEMPLATE("task_template"),
    USER_SETTINGS("user_settings"),
    USER_STATE("user_state"),
    APP_SETTINGS("app_settings"),
    ;

    companion object {
        private val byStored: Map<String, ReplicaEntityKind> = entries.associateBy { it.stored }

        /**
         * The kind a stored or received name denotes, or `null` when this build
         * has never heard of it. A caller decides what to do with an unknown
         * kind; there is no sentinel, because there is nothing sensible a
         * replica can do with a record it has no table for.
         */
        fun fromStored(value: String?): ReplicaEntityKind? = byStored[value]
    }
}
