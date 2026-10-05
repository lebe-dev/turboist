# Rules this module carries into whatever app is built from it.
#
# A minified build removes and renames everything nothing points at. Everything
# below is found by name at runtime instead, so nothing points at it in a way the
# shrinker can see, and without these rules it disappears from a release build
# while every debug build keeps working.

# Retrofit builds an endpoint's implementation from the annotations on the
# interface, read off the Class object at the moment the endpoint is asked for.
# There is no compiled reference to the interface's members at all.
-keep interface ru.tinyops.turboist.core.network.api.** { *; }

# The generic return type is how the converter knows what to parse a response
# into, and the annotations are the request itself. Both are attributes the
# shrinker strips unless asked not to.
-keepattributes Signature, InnerClasses, EnclosingMethod
-keepattributes RuntimeVisibleAnnotations, RuntimeVisibleParameterAnnotations, AnnotationDefault

# The wire types. Their JSON codec is generated beside each class and reached
# through the class, and a field's name is the key it is written under.
-keep class ru.tinyops.turboist.core.network.dto.** { *; }

# The module root holds the error envelope every failed response is read through
# and the tri-state value a patch writes an explicit null with. Both are wire
# types that happen not to live under `dto`.
-keep class ru.tinyops.turboist.core.network.* { *; }

# The generated serializers, and the companion object each one is reached
# through. Kept for every wire type above.
-keep class ru.tinyops.turboist.core.network.**$$serializer { *; }
-keepclassmembers class ru.tinyops.turboist.core.network.** {
    *** Companion;
}
