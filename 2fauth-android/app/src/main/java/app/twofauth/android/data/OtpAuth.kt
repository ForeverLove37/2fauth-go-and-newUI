package app.twofauth.android.data

import android.net.Uri
import android.util.Base64

/** The fields of an account read from a QR code or a link, before it is saved anywhere. */
data class ParsedAccount(
    val service: String,
    val account: String,
    val secret: String,
    val otpType: String = "totp",
    val digits: Int = 6,
    val algorithm: String = "sha1",
    val period: Int = 30,
    val counter: Long = 0,
)

/**
 * Reads otpauth:// links and Google Authenticator exports on the phone, so that a code
 * meant to stay on it never passes through a server, not even to be decoded.
 */
object OtpAuth {
    class FormatException(message: String) : Exception(message)

    fun parse(text: String): List<ParsedAccount> {
        val value = text.trim()
        return when {
            value.startsWith("otpauth-migration://", true) -> migration(value)
            value.startsWith("otpauth://", true) -> listOf(single(value))
            else -> throw FormatException("not an otpauth link")
        }
    }

    private fun single(text: String): ParsedAccount {
        val uri = Uri.parse(text)
        val host = uri.host?.lowercase()
        if (host != "totp" && host != "hotp" && host != "steam") throw FormatException("unknown type $host")
        val label = Uri.decode(uri.path.orEmpty().trimStart('/'))
        val issuerFromLabel = label.substringBefore(':', "").trim()
        val name = label.substringAfter(':').trim()
        val issuer = uri.getQueryParameter("issuer")?.trim()?.takeIf { it.isNotEmpty() } ?: issuerFromLabel
        val secret = uri.getQueryParameter("secret")?.replace(" ", "")?.uppercase()
            ?: throw FormatException("no secret")
        if (!OtpEngine.isValidSecret(secret)) throw FormatException("invalid secret")
        val steam = host == "steam" || uri.getQueryParameter("encoder") == "steam" || issuer.equals("Steam", true) && host == "totp" && uri.getQueryParameter("digits") == "5"
        val type = when {
            steam -> "steamtotp"
            host == "hotp" -> "hotp"
            else -> "totp"
        }
        return ParsedAccount(
            service = issuer,
            account = name,
            secret = secret,
            otpType = type,
            digits = if (steam) 5 else uri.getQueryParameter("digits")?.toIntOrNull()?.coerceIn(5, 10) ?: 6,
            algorithm = (uri.getQueryParameter("algorithm") ?: "sha1").lowercase().takeIf { it in setOf("sha1", "sha256", "sha512", "md5") } ?: "sha1",
            period = uri.getQueryParameter("period")?.toIntOrNull()?.takeIf { it > 0 } ?: 30,
            counter = uri.getQueryParameter("counter")?.toLongOrNull() ?: 0,
        )
    }

    /** otpauth-migration://offline?data=… : a protobuf list of accounts (Google Authenticator export). */
    private fun migration(text: String): List<ParsedAccount> {
        val data = Uri.parse(text).getQueryParameter("data") ?: throw FormatException("no data")
        val bytes = Base64.decode(data.replace(' ', '+'), Base64.DEFAULT)
        val accounts = mutableListOf<ParsedAccount>()
        Protobuf(bytes).fields { number, value ->
            if (number == 1 && value is ByteArray) accounts += migrationEntry(value)
        }
        if (accounts.isEmpty()) throw FormatException("empty export")
        return accounts
    }

    private fun migrationEntry(bytes: ByteArray): ParsedAccount {
        var secret = ByteArray(0)
        var name = ""
        var issuer = ""
        var algorithm = 1L
        var digits = 1L
        var type = 2L
        var counter = 0L
        Protobuf(bytes).fields { number, value ->
            when (number) {
                1 -> secret = value as ByteArray
                2 -> name = String(value as ByteArray, Charsets.UTF_8)
                3 -> issuer = String(value as ByteArray, Charsets.UTF_8)
                4 -> algorithm = value as Long
                5 -> digits = value as Long
                6 -> type = value as Long
                7 -> counter = value as Long
            }
        }
        val label = name.substringAfter(':').trim()
        val service = issuer.ifEmpty { name.substringBefore(':', "").trim() }
        return ParsedAccount(
            service = service,
            account = label,
            secret = base32Encode(secret),
            otpType = if (type == 1L) "hotp" else "totp",
            digits = if (digits == 2L) 8 else 6,
            algorithm = when (algorithm) { 2L -> "sha256"; 3L -> "sha512"; 4L -> "md5"; else -> "sha1" },
            counter = counter,
        )
    }

    fun base32Encode(bytes: ByteArray): String {
        val alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
        val out = StringBuilder()
        var buffer = 0
        var bits = 0
        for (byte in bytes) {
            buffer = (buffer shl 8) or (byte.toInt() and 0xFF)
            bits += 8
            while (bits >= 5) {
                out.append(alphabet[(buffer shr (bits - 5)) and 31])
                bits -= 5
            }
        }
        if (bits > 0) out.append(alphabet[(buffer shl (5 - bits)) and 31])
        return out.toString()
    }

    /** The link of an account, to hand it to the server when it gets synced. */
    fun uri(a: LocalAccount): String {
        val type = if (a.otpType == "hotp") "hotp" else "totp"
        val label = Uri.encode(if (a.service.isNotEmpty()) "${a.service}:${a.account}" else a.account)
        return buildString {
            append("otpauth://$type/$label?secret=${a.secret}")
            if (a.service.isNotEmpty()) append("&issuer=${Uri.encode(a.service)}")
            append("&algorithm=${a.algorithm.uppercase()}&digits=${a.digits}")
            if (type == "hotp") append("&counter=${a.counter}") else append("&period=${a.period}")
        }
    }
}

/** Just enough protobuf to read the export of Google Authenticator. */
private class Protobuf(private val bytes: ByteArray) {
    private var position = 0

    private fun varint(): Long {
        var result = 0L
        var shift = 0
        while (true) {
            val byte = bytes[position++].toInt() and 0xFF
            result = result or ((byte and 0x7F).toLong() shl shift)
            if (byte and 0x80 == 0) return result
            shift += 7
        }
    }

    fun fields(each: (Int, Any) -> Unit) {
        while (position < bytes.size) {
            val key = varint()
            val number = (key shr 3).toInt()
            when ((key and 7).toInt()) {
                0 -> each(number, varint())
                2 -> {
                    val length = varint().toInt()
                    each(number, bytes.copyOfRange(position, position + length))
                    position += length
                }
                1 -> position += 8
                5 -> position += 4
                else -> return
            }
        }
    }
}
