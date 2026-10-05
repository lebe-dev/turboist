# Rules this module carries into whatever app is built from it.
#
# A queued write is stored as JSON in the replica and may still be sitting there
# when the app is updated, so the build that reads it back is not the build that
# wrote it. That makes the op catalog a persistence format rather than an
# in-process detail: every class in it has to keep the shape and the name it was
# serialized under, or an unsent change made before an update is unreadable
# afterwards and the user silently loses it.
-keep class ru.tinyops.turboist.core.sync.write.** { *; }
-keep class ru.tinyops.turboist.core.sync.write.**$$serializer { *; }
-keepclassmembers class ru.tinyops.turboist.core.sync.write.** {
    *** Companion;
}
