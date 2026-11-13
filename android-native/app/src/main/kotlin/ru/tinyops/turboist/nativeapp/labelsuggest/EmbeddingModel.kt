package ru.tinyops.turboist.nativeapp.labelsuggest

/**
 * The language model label suggestions run on: EmbeddingGemma 2, text only,
 * in the LiteRT-LM format, published under Apache 2.0.
 *
 * Not shipped in the app — it is several times the size of everything else in
 * it, and only someone who turns suggestions on needs it — but fetched once
 * from the public model hub when they do.
 *
 * The address names a fixed revision rather than the latest, and the file is
 * checked against [SHA256] before it is used, so what runs on the device is
 * byte for byte the file the suggestions were calibrated on. Moving to a new
 * model means new numbers in the recipe, not just a new address here.
 */
internal object EmbeddingModel {
    /** Part of every cached embedding's key, so another model never reads this one's vectors. */
    const val ID: String = "embeddinggemma-2-text-270m@9be6e8b9"

    const val FILE_NAME: String = "embeddinggemma-2-text-270m.litertlm"

    const val URL: String =
        "https://huggingface.co/litert-community/embeddinggemma-2-text-270m-litert-lm/resolve/" +
            "9be6e8b90982095dc05c2bd162e4b954ee4dbac7/embeddinggemma-2-text-270m.litertlm"

    const val SHA256: String = "2d079ee2f6f066b1f368e8d7c819f55214eaef1d0513b312321901f30ab286fb"

    const val SIZE_BYTES: Long = 164_626_432L

    /**
     * The longest input the engine is opened for, in tokens. Titles are far
     * shorter; this is for a label carrying a long description and many projects.
     */
    const val MAX_INPUT_TOKENS: Int = 512

    /**
     * Texts are cut to this many characters before they reach the engine, so an
     * unusually long description is shortened here rather than refused there.
     * Comfortably under [MAX_INPUT_TOKENS] for Russian and English alike.
     */
    const val MAX_INPUT_CHARS: Int = 1_200
}

/**
 * Whether this phone can run the model at all.
 *
 * The engine ships its native library for 64-bit ARM and x86 only, while the app
 * itself still installs on 32-bit phones. Rather than offer a switch whose model
 * would download and then fail to load, the feature is not offered there at all.
 */
internal object LabelSuggestionSupport {
    private val engineAbis = setOf("arm64-v8a", "x86_64")

    val supported: Boolean
        get() = android.os.Build.SUPPORTED_ABIS.orEmpty().any { it in engineAbis }
}
