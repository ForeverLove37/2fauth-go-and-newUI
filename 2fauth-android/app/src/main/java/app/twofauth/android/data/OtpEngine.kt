package app.twofauth.android.data

import java.nio.ByteBuffer
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

/**
 * Codes computed on the phone, for the accounts whose secret it keeps: RFC 4226 (HOTP),
 * RFC 6238 (TOTP) and the five letter codes of Steam Guard.
 */
object OtpEngine {
    private const val STEAM_ALPHABET = "23456789BCDFGHJKMNPQRTVWXY"

    /** Base32 without padding, spaces or case: what services print under their QR codes. */
    fun base32(secret: String): ByteArray {
        val clean = secret.uppercase().filter { it != ' ' && it != '-' && it != '=' }
        require(clean.isNotEmpty()) { "empty secret" }
        val out = java.io.ByteArrayOutputStream()
        var buffer = 0
        var bits = 0
        for (char in clean) {
            val value = when (char) {
                in 'A'..'Z' -> char - 'A'
                in '2'..'7' -> char - '2' + 26
                else -> throw IllegalArgumentException("not base32: $char")
            }
            buffer = (buffer shl 5) or value
            bits += 5
            if (bits >= 8) {
                out.write((buffer shr (bits - 8)) and 0xFF)
                bits -= 8
            }
        }
        return out.toByteArray()
    }

    fun isValidSecret(secret: String): Boolean = runCatching { base32(secret).isNotEmpty() }.getOrDefault(false)

    private fun mac(algorithm: String): String = when (algorithm.lowercase()) {
        "sha256" -> "HmacSHA256"
        "sha512" -> "HmacSHA512"
        "md5" -> "HmacMD5"
        else -> "HmacSHA1"
    }

    private fun truncated(key: ByteArray, counter: Long, algorithm: String): Int {
        val name = mac(algorithm)
        val hash = Mac.getInstance(name).run {
            init(SecretKeySpec(key, name))
            doFinal(ByteBuffer.allocate(8).putLong(counter).array())
        }
        val offset = hash[hash.size - 1].toInt() and 0x0F
        return ((hash[offset].toInt() and 0x7F) shl 24) or
            ((hash[offset + 1].toInt() and 0xFF) shl 16) or
            ((hash[offset + 2].toInt() and 0xFF) shl 8) or
            (hash[offset + 3].toInt() and 0xFF)
    }

    fun hotp(secret: String, counter: Long, digits: Int = 6, algorithm: String = "sha1"): String {
        val value = truncated(base32(secret), counter, algorithm)
        var modulo = 1L
        repeat(digits.coerceIn(1, 10)) { modulo *= 10 }
        return (value % modulo).toString().padStart(digits, '0')
    }

    fun steam(secret: String, counter: Long): String {
        var value = truncated(base32(secret), counter, "sha1")
        return buildString {
            repeat(5) {
                append(STEAM_ALPHABET[value % STEAM_ALPHABET.length])
                value /= STEAM_ALPHABET.length
            }
        }
    }

    /** The code of a time based account at a moment, or "" when its secret is unusable. */
    fun totpAt(secret: String, epochSeconds: Long, period: Int, digits: Int, algorithm: String, steam: Boolean = false): String =
        runCatching {
            val counter = epochSeconds / period.coerceAtLeast(1)
            if (steam) steam(secret, counter) else hotp(secret, counter, digits, algorithm)
        }.getOrDefault("")
}
