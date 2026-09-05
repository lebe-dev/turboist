package ru.tinyops.turboist.core.sync.drain

/**
 * How long to leave a stalled queue alone before trying it again.
 *
 * The delay grows with each consecutive failure and then stops growing. Growing
 * matters because the two things that stall a queue — no network and a server
 * that is unwell — both last minutes rather than milliseconds, and a phone that
 * retries every second through either of them spends its battery on nothing. The
 * ceiling matters because a queue that has waited a quarter of an hour must not
 * then wait an hour: the user is owed an attempt soon after conditions change,
 * and something usually tells the app when they have.
 *
 * There is no jitter. A single user with a single server has no thundering herd
 * to spread out, and a delay that cannot be predicted is a delay that cannot be
 * asserted on.
 */
fun interface DrainBackoff {
    /**
     * The wait after [attempt] consecutive failures, where the first failure is
     * attempt 1.
     */
    fun delayAfter(attempt: Int): Long

    companion object {
        /** The first wait: long enough not to be a busy loop, short enough to be unnoticed. */
        const val BASE_DELAY_MILLIS: Long = 2_000L

        /** The longest wait. A trigger — a network arriving, the app coming back — cuts it short. */
        const val MAX_DELAY_MILLIS: Long = 5 * 60_000L

        /** Doubling from [BASE_DELAY_MILLIS] up to [MAX_DELAY_MILLIS]. */
        val Default: DrainBackoff =
            DrainBackoff { attempt ->
                if (attempt <= 1) {
                    BASE_DELAY_MILLIS
                } else {
                    val doublings = (attempt - 1).coerceAtMost(MAX_DOUBLINGS)
                    (BASE_DELAY_MILLIS shl doublings).coerceAtMost(MAX_DELAY_MILLIS)
                }
            }

        /**
         * Where doubling is capped before the multiplication itself could
         * overflow. The ceiling is reached long before this.
         */
        private const val MAX_DOUBLINGS: Int = 16
    }
}
