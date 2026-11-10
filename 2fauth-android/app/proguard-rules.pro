# kotlinx.serialization keeps what it needs through its own rules;
# these cover the models read by reflection-free generated serializers.
-keepattributes *Annotation*, InnerClasses
-keep,includedescriptorclasses class app.twofauth.android.**$$serializer { *; }
-keepclassmembers class app.twofauth.android.** {
    *** Companion;
}
-keepclasseswithmembers class app.twofauth.android.** {
    kotlinx.serialization.KSerializer serializer(...);
}
-dontwarn okhttp3.internal.platform.**
-dontwarn org.conscrypt.**
-dontwarn org.bouncycastle.**
-dontwarn org.openjsse.**
