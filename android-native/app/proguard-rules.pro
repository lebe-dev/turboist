# Shrinker rules for the app itself. Each module contributes its own rules for
# what it declares; this file covers only what the app module declares.
#
# What all of them have in common: the class is reached by name at runtime, so
# nothing points at it in a way the shrinker can see, and a release build would
# quietly do without it while every debug build kept working.

# A destination is identified by its serialized form: that is what is written
# into the back stack, restored from saved state after the process is killed,
# and matched against an incoming link. Renaming one changes the identity of
# every screen the user had open.
-keep class ru.tinyops.turboist.nativeapp.navigation.** { *; }

# The external calendar is cached on disk as JSON and read back by a later run
# of the app, which may be a later build of it.
-keep class ru.tinyops.turboist.nativeapp.calendar.** { *; }

# The generated serializers for both, and the companion object each is reached
# through.
-keep class ru.tinyops.turboist.nativeapp.**$$serializer { *; }
-keepclassmembers class ru.tinyops.turboist.nativeapp.** {
    *** Companion;
}

# The stack traces that come back from a release build are otherwise a list of
# single-letter names. This keeps enough of the original ones to read them, and
# costs only the size of the tables.
-keepattributes SourceFile, LineNumberTable
-renamesourcefileattribute SourceFile
