package ru.tinyops.turboist.core.model.labelsuggest

/**
 * The numbers project suggestions are scored with, in one place.
 *
 * Measured the same way as [LabelSuggestionRecipe]: the shipped model over a
 * real workspace of about 3,100 tasks in 80 projects, the newest fifth held out,
 * each held-out task seeing only what was written before it, and scored against
 * the project its owner actually filed it into. Change one by measuring again,
 * not by reasoning about it.
 *
 * Three signals are blended, each a number from 0 to 1 per project:
 *
 * - the neighbours: the most similar earlier tasks vote for the project they
 *   sit in, each run of a repeating task counted once. As with labels, a
 *   workspace's own history carries the score;
 * - the centroid: how close the title is to what each project's recent tasks
 *   read like on average, relative to every other project. The neighbours see a
 *   few tasks; the centroid sees a project's whole recent run of them;
 * - the project itself: how close the title is to each project's name and
 *   description. Alone it is nearly useless — project names are proper nouns
 *   the model has never met — but it is what a project with little history has.
 */
object ProjectSuggestionRecipe {
    /** How many earlier tasks are asked. */
    const val NEIGHBOURS: Int = 20

    /**
     * How sharply a closer neighbour outvotes a farther one. Much sharper than
     * for labels: short titles all sit at cosines between about 0.84 and 0.9,
     * where a fourth power cannot tell a task that matches exactly from a crowd
     * of loosely related ones, and the crowd then names the wrong project.
     */
    const val NEIGHBOUR_SHARPNESS: Int = 16

    /**
     * After how long a neighbour's vote counts half. Shorter than for labels:
     * projects come and go faster than what a label means.
     */
    const val HALF_LIFE_MILLIS: Long = 60L * 24 * 60 * 60 * 1000

    /** How many of a project's newest tasks its centroid is averaged from. */
    const val CENTROID_TASKS: Int = 50

    /** How closeness to centroids and to project documents is turned into shares. */
    const val SOFTMAX_TEMPERATURE: Float = 0.02f

    const val WEIGHT_NEIGHBOURS: Float = 0.5f
    const val WEIGHT_CENTROIDS: Float = 0.4f
    const val WEIGHT_DOCUMENTS: Float = 0.1f

    /**
     * The score a project needs to be offered in the capture sheet. At this bar
     * about six chips in seven were the project the task actually went to, and
     * more than half of the tasks got the right one among their chips.
     */
    const val CAPTURE_THRESHOLD: Float = 0.3f

    /** The most projects offered at once. */
    const val MAX_SUGGESTIONS: Int = 3
}
