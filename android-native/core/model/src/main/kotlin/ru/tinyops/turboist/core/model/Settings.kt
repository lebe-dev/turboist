package ru.tinyops.turboist.core.model

/**
 * The pinning caps a user may set, and the value an absent or out-of-range one
 * falls back to. Zero is deliberately not "no limit": it would read as "pinning
 * is off", which is not a state the product has.
 */
const val DEFAULT_MAX_PINNED: Int = 10
const val MIN_MAX_PINNED: Int = 1
const val MAX_MAX_PINNED: Int = 50

/** How many entities the harpoon holds. Attaching a third evicts the first. */
const val HARPOON_SLOTS: Int = 2

/**
 * Brings a pinning cap into range, mirroring what the server does when it reads a
 * settings blob written before the caps existed. Out of range becomes the
 * default rather than the nearest bound: a stored 0 is not a user's wish for
 * "one", it is a blob that predates the setting.
 */
fun normalizeMaxPinned(value: Int?): Int =
    if (value == null || value < MIN_MAX_PINNED || value > MAX_MAX_PINNED) DEFAULT_MAX_PINNED else value

/**
 * One harpooned entity: the "jump pair" the user hops between.
 *
 * @property id a **server** id. The harpoon is stored inside the settings blob
 *   and echoed back to the server verbatim on write, so rewriting it to a local
 *   id would corrupt it. Resolve it against the replica when rendering.
 */
data class HarpoonRef(
    val kind: HarpoonKind,
    val id: Long,
)

/**
 * The user's own preferences, a single blob on the server rather than a table.
 *
 * Every label id in here is a **server** id, for the same reason as
 * [HarpoonRef.id]: these lists travel back to the server unchanged.
 *
 * @property bannerDayPart narrows the Today banner to one phase of the day.
 *   `null` means all day — the server spells that as an empty string, which is
 *   why this is nullable rather than [DayPart.NONE].
 * @property calendarHidePastEvents defaults to on, matching what the server fills
 *   in for a blob written before the setting existed.
 * @property harpoon the ordered jump pair; slot order is significant.
 */
data class UserSettings(
    val weeklyUnplannedExcludedLabelIds: List<Long> = emptyList(),
    val bugLabelIds: List<Long> = emptyList(),
    val locale: String = "",
    val publicView: Boolean = false,
    val bannerText: String = "",
    val bannerPublished: Boolean = false,
    val bannerDayPart: DayPart? = null,
    val calendarEnabled: Boolean = false,
    val calendarHidePastEvents: Boolean = true,
    val troikiEnabled: Boolean = false,
    val maxPinnedTasks: Int = DEFAULT_MAX_PINNED,
    val maxPinnedProjects: Int = DEFAULT_MAX_PINNED,
    val harpoon: List<HarpoonRef> = emptyList(),
)
