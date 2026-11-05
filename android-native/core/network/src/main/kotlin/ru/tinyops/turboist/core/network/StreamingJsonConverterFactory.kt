package ru.tinyops.turboist.core.network

import kotlinx.serialization.ExperimentalSerializationApi
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.okio.decodeFromBufferedSource
import kotlinx.serialization.serializer
import okhttp3.MediaType
import okhttp3.RequestBody
import okhttp3.ResponseBody
import retrofit2.Converter
import retrofit2.Retrofit
import retrofit2.converter.kotlinx.serialization.asConverterFactory
import java.lang.reflect.Type

/**
 * Decodes a response as it arrives instead of reading it into one string first.
 *
 * A complete copy of the workspace is a single document with no paging. Read
 * whole, it exists twice at the peak of the decode: once as a string as long as
 * the response and once as the objects being built from it, and it is the string
 * that takes the most contiguous memory on the phones with the least of it.
 *
 * Only responses change. Request bodies are small and written the stock way, so
 * what goes on the wire cannot differ because of this.
 */
internal class StreamingJsonConverterFactory(
    private val json: Json,
    contentType: MediaType,
) : Converter.Factory() {
    private val stock: Converter.Factory = json.asConverterFactory(contentType)

    @OptIn(ExperimentalSerializationApi::class)
    override fun responseBodyConverter(
        type: Type,
        annotations: Array<out Annotation>,
        retrofit: Retrofit,
    ): Converter<ResponseBody, *> {
        val deserializer = json.serializersModule.serializer(type)
        return Converter<ResponseBody, Any?> { body ->
            body.use { json.decodeFromBufferedSource(deserializer, it.source()) }
        }
    }

    override fun requestBodyConverter(
        type: Type,
        parameterAnnotations: Array<out Annotation>,
        methodAnnotations: Array<out Annotation>,
        retrofit: Retrofit,
    ): Converter<*, RequestBody>? = stock.requestBodyConverter(type, parameterAnnotations, methodAnnotations, retrofit)
}
