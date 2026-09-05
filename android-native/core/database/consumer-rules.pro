# Rules this module carries into whatever app is built from it.
#
# The replica's implementation is written by the annotation processor beside the
# class it was generated from and found again by appending a suffix to that
# class's name. That lookup is a string, so the shrinker sees nothing pointing at
# the generated class and is free to delete it; the app then fails to open its
# own database on first launch, in release builds only.
-keep class ru.tinyops.turboist.core.database.TurboistDatabase { *; }
-keep class ru.tinyops.turboist.core.database.*_Impl { *; }
