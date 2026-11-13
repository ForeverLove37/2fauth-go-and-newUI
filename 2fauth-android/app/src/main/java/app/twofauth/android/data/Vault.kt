package app.twofauth.android.data

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Small values kept encrypted with a key that never leaves the Android Keystore:
 * a copy of the preferences file (a backup, a rooted reader) holds nothing usable.
 */
class Vault(context: Context) {
    private val prefs = context.getSharedPreferences("vault", Context.MODE_PRIVATE)

    fun get(name: String): String? {
        val stored = prefs.getString(name, null) ?: return null
        return runCatching {
            val raw = Base64.decode(stored, Base64.NO_WRAP)
            val cipher = Cipher.getInstance(TRANSFORMATION)
            cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, raw, 0, IV_SIZE))
            String(cipher.doFinal(raw, IV_SIZE, raw.size - IV_SIZE), Charsets.UTF_8)
        }.getOrNull()
    }

    fun put(name: String, value: String?) {
        if (value == null) {
            prefs.edit().remove(name).apply()
            return
        }
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        val sealed = cipher.iv + cipher.doFinal(value.toByteArray(Charsets.UTF_8))
        prefs.edit().putString(name, Base64.encodeToString(sealed, Base64.NO_WRAP)).apply()
    }

    fun clear() = prefs.edit().clear().apply()

    /** Encrypts a whole file's worth of bytes with the same key. */
    fun seal(plain: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.ENCRYPT_MODE, key())
        return cipher.iv + cipher.doFinal(plain)
    }

    fun open(sealed: ByteArray): ByteArray {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, sealed, 0, IV_SIZE))
        return cipher.doFinal(sealed, IV_SIZE, sealed.size - IV_SIZE)
    }

    private fun key(): SecretKey {
        val store = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (store.getKey(ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(ALIAS, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .build(),
        )
        return generator.generateKey()
    }

    private companion object {
        const val KEYSTORE = "AndroidKeyStore"
        const val ALIAS = "2fauth-vault"
        const val TRANSFORMATION = "AES/GCM/NoPadding"
        const val IV_SIZE = 12
    }
}

/** The local choices of this phone, none of them secret. */
class LocalSettings(context: Context) {
    private val prefs = context.getSharedPreferences("settings", Context.MODE_PRIVATE)

    var appLock: Boolean
        get() = prefs.getBoolean("app_lock", true)
        set(value) = prefs.edit().putBoolean("app_lock", value).apply()

    /** Seconds in the background after which the app asks to be unlocked again. */
    var lockAfter: Int
        get() = prefs.getInt("lock_after", 60)
        set(value) = prefs.edit().putInt("lock_after", value).apply()

    var blockScreenshots: Boolean
        get() = prefs.getBoolean("block_screenshots", true)
        set(value) = prefs.edit().putBoolean("block_screenshots", value).apply()

    /** Seconds after which a copied code is wiped from the clipboard, 0 to keep it. */
    var clipboardClear: Int
        get() = prefs.getInt("clipboard_clear", 30)
        set(value) = prefs.edit().putInt("clipboard_clear", value).apply()

    /** "123 456" instead of "123456": easier to read off a phone and type elsewhere. */
    var splitDigits: Boolean
        get() = prefs.getBoolean("split_digits", true)
        set(value) = prefs.edit().putBoolean("split_digits", value).apply()

    /** Where a new code is saved: on the phone only (default) or synced with the server. */
    var defaultKind: Kind
        get() = runCatching { Kind.valueOf(prefs.getString("default_kind", "LOCAL")!!) }.getOrDefault(Kind.LOCAL)
        set(value) = prefs.edit().putString("default_kind", value.name).apply()

    var hideCodes: Boolean
        get() = prefs.getBoolean("hide_codes", true)
        set(value) = prefs.edit().putBoolean("hide_codes", value).apply()

    var showNext: Boolean
        get() = prefs.getBoolean("show_next", true)
        set(value) = prefs.edit().putBoolean("show_next", value).apply()

    var showIcons: Boolean
        get() = prefs.getBoolean("show_icons", true)
        set(value) = prefs.edit().putBoolean("show_icons", value).apply()

    /** Logos of phone-only codes from the public icon libraries (they see the service name). */
    var publicIcons: Boolean
        get() = prefs.getBoolean("public_icons", true)
        set(value) = prefs.edit().putBoolean("public_icons", value).apply()

    var themeId: String
        get() = prefs.getString("theme", "deep-space") ?: "deep-space"
        set(value) = prefs.edit().putString("theme", value).apply()

    /** 0 keeps the accent of the theme. */
    var accent: Long
        get() = prefs.getLong("accent", 0L)
        set(value) = prefs.edit().putLong("accent", value).apply()

    var cardStyle: String
        get() = prefs.getString("card_style", "Classic") ?: "Classic"
        set(value) = prefs.edit().putString("card_style", value).apply()

    var animations: Boolean
        get() = prefs.getBoolean("animations", true)
        set(value) = prefs.edit().putBoolean("animations", value).apply()

    var checkUpdates: Boolean
        get() = prefs.getBoolean("check_updates", true)
        set(value) = prefs.edit().putBoolean("check_updates", value).apply()

    var lastServer: String
        get() = prefs.getString("last_server", "") ?: ""
        set(value) = prefs.edit().putString("last_server", value).apply()
}
