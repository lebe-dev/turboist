package ru.tinyops.turboist.core.model.labelsuggest

/**
 * The numbers label suggestions are scored with, in one place.
 *
 * None of them is a guess. They were chosen by running the shipped model over a
 * real workspace of a few thousand tasks — the newest fifth held out and scored
 * against the labels its owner actually gave it — and they are the point where
 * a chip offered while typing is right about four times out of five. Changing
 * one changes how often the capture sheet speaks up and how often it is wrong,
 * so a change should be measured the same way rather than reasoned about.
 *
 * Four signals are blended, each already a number from 0 to 1 per label:
 *
 * - the neighbours: the most similar earlier tasks vote with their labels. This
 *   is what carries the score — a workspace's own history knows what its labels
 *   mean far better than their names do;
 * - the label itself: how close the title is to the label's name, description
 *   and the projects that carry it, relative to every other label;
 * - the projects: how close the title is to each project's name and description,
 *   each project then voting for the labels it stands for;
 * - the destination: when the task already has a project, the labels its other
 *   tasks were given beyond the project's own.
 */
object LabelSuggestionRecipe {
    /**
     * How much of each embedding is kept. The model's vectors are built so a
     * prefix is itself a usable embedding; past this length the extra numbers
     * bought nothing measurable and would quadruple what the device stores.
     */
    const val DIMENSIONS: Int = 256

    /** How many earlier tasks are asked. */
    const val NEIGHBOURS: Int = 20

    /**
     * How sharply a closer neighbour outvotes a farther one: each vote is its
     * similarity raised to this power, so the one nearly identical task beats
     * a crowd of loosely related ones.
     */
    const val NEIGHBOUR_SHARPNESS: Int = 4

    /**
     * After how long a neighbour's vote counts half. Labels drift — what a
     * label was used for a year ago is weaker evidence than last month.
     */
    const val HALF_LIFE_MILLIS: Long = 180L * 24 * 60 * 60 * 1000

    /**
     * How the closeness to each label (and to each project) is turned into
     * shares that sum to one. Raw closeness is useless as a score: every short
     * title is roughly as close to every label, so only how much closer it is
     * to one than to the others says anything.
     */
    const val SOFTMAX_TEMPERATURE: Float = 0.02f

    const val WEIGHT_NEIGHBOURS: Float = 0.5f
    const val WEIGHT_LABELS: Float = 0.3f
    const val WEIGHT_PROJECTS: Float = 0.1f
    const val WEIGHT_DESTINATION: Float = 0.1f

    /**
     * The neighbours' weight while the task has no project: the destination's
     * share has nothing to say, and handing it to the neighbours scored better
     * than spreading it.
     */
    const val WEIGHT_NEIGHBOURS_WITHOUT_PROJECT: Float = WEIGHT_NEIGHBOURS + WEIGHT_DESTINATION

    /**
     * A label counts as one a project stands for, beyond the project's own,
     * once at least this share of the project's tasks carries it.
     */
    const val PROJECT_FREQUENT_SHARE: Float = 0.3f

    /**
     * How many tasks' worth of doubt the destination signal starts with: a
     * project with two tasks that both carry a label is not yet evidence that
     * the third will.
     */
    const val DESTINATION_SHRINK: Int = 5

    /** The score a chip needs to be offered unasked, while the title is typed. */
    const val CAPTURE_THRESHOLD: Float = 0.4f

    /**
     * The score a label needs to lead the label picker. Low on purpose: the
     * user is already choosing a label there, so ranking matters and a miss
     * costs nothing but a glance.
     */
    const val PICKER_FLOOR: Float = 0.05f

    /** The most labels offered at once. */
    const val MAX_SUGGESTIONS: Int = 3
}
