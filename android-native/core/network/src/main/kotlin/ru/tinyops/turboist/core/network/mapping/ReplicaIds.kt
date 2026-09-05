package ru.tinyops.turboist.core.network.mapping

/**
 * Translates the server ids inside a payload into the local ids the replica
 * references rows by.
 *
 * A row created on the device exists — and is referenced by open screens — before
 * the server has ever heard of it, so the replica assigns its own identity and
 * treats the server id as a second, later one. Every reference between replicated
 * rows is therefore local, and something has to look those up while a wire
 * payload is being turned into domain objects. That something is the replica; this
 * interface is the hole it plugs into, so the mapping code needs no database.
 *
 * A `null` answer means "no such row here yet", which is a normal state during a
 * pull: pages are applied in log order, and a task can name a project whose own
 * change is still two pages away.
 */
interface ReplicaIds {
    fun task(serverId: Long?): Long?

    fun project(serverId: Long?): Long?

    fun section(serverId: Long?): Long?

    fun context(serverId: Long?): Long?

    fun label(serverId: Long?): Long?

    fun template(serverId: Long?): Long?

    companion object {
        /**
         * Local ids *are* the server's ids.
         *
         * Correct for any consumer that never creates rows offline — and the
         * simplest thing that lets a mapping be tested without a database.
         */
        val Identity: ReplicaIds =
            object : ReplicaIds {
                override fun task(serverId: Long?): Long? = serverId

                override fun project(serverId: Long?): Long? = serverId

                override fun section(serverId: Long?): Long? = serverId

                override fun context(serverId: Long?): Long? = serverId

                override fun label(serverId: Long?): Long? = serverId

                override fun template(serverId: Long?): Long? = serverId
            }
    }
}
