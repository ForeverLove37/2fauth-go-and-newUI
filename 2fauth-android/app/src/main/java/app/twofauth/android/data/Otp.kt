package app.twofauth.android.data

import android.content.ClipData
import android.content.ClipDescription
import android.content.ClipboardManager
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.PersistableBundle

/** Seconds left in the current window of a TOTP account. */
fun Account.remainingSeconds(nowMillis: Long): Int {
    val period = period ?: return 0
    if (period <= 0) return 0
    return period - ((nowMillis / 1000) % period).toInt()
}

/**
 * The code valid now. The server sends the code of the next window too, so the
 * card stays right when the window rolls over before the next refresh.
 */
fun Account.currentCode(nowMillis: Long): String {
    val payload = otp ?: return ""
    if (payload.password.isEmpty()) return ""
    val period = period ?: return payload.password
    val generated = payload.generatedAt ?: return payload.password
    val next = payload.nextPassword ?: return payload.password
    if (period <= 0) return payload.password
    val window = nowMillis / 1000 / period
    val generatedWindow = generated / period
    return when {
        window == generatedWindow -> payload.password
        window == generatedWindow + 1 -> next
        else -> "" // both codes have expired: a refresh is due
    }
}

/** The code of the window after the current one, when it is already known. */
fun Account.nextCode(nowMillis: Long): String {
    val payload = otp ?: return ""
    val period = period ?: return ""
    val generated = payload.generatedAt ?: return ""
    if (period <= 0) return ""
    return if (nowMillis / 1000 / period == generated / period) payload.nextPassword.orEmpty() else ""
}

/** True once the current window is the one of next_password: time to fetch more codes. */
fun Account.needsRefresh(nowMillis: Long): Boolean {
    if (isHotp) return false
    val payload = otp ?: return true
    val period = period ?: return false
    val generated = payload.generatedAt ?: return false
    if (period <= 0) return false
    return nowMillis / 1000 / period > generated / period
}

fun Account.label(): String = service?.takeIf { it.isNotBlank() } ?: account?.takeIf { it.isNotBlank() } ?: "#$id"

fun Account.subLabel(): String = when {
    !service.isNullOrBlank() && !account.isNullOrBlank() -> account
    isBorrowed -> borrowedBy.orEmpty()
    else -> otpType.uppercase()
}

fun Account.initials(): String {
    val source = service?.takeIf { it.isNotBlank() } ?: account?.takeIf { it.isNotBlank() } ?: "?"
    val words = source.replace(Regex("[^\\p{L}\\p{N}\\s]"), " ").trim().split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (words.size >= 2) return (words[0].take(1) + words[1].take(1)).uppercase()
    return source.take(2).uppercase()
}

/** The same hue per service as the web panel, so a service has one colour everywhere. */
fun Account.identityHue(): Float {
    val source = (service?.takeIf { it.isNotEmpty() } ?: account?.takeIf { it.isNotEmpty() } ?: id.toString()).lowercase()
    var hash = 0L
    // JavaScript iterates UTF-16 code units, and so does a Kotlin String.
    for (char in source) hash = (hash * 31 + char.code) and 0xFFFFFFFFL
    return Math.round((hash * 0.61803398875 * 360) % 360).toFloat()
}

/** "123456" → "123 456", "12345678" → "1234 5678". */
fun formatCode(code: String, split: Boolean): String {
    if (!split || code.length < 6 || code.any { !it.isDigit() }) return code
    val half = (code.length + 1) / 2
    return code.substring(0, half) + " " + code.substring(half)
}

/**
 * Copies a code marked as sensitive, so Android does not show it in the clipboard
 * preview, and wipes it after a while unless something else has been copied since.
 */
object CodeClipboard {
    private val handler = Handler(Looper.getMainLooper())
    private var pending: Runnable? = null

    fun copy(context: Context, code: String, clearAfterSeconds: Int) {
        val manager = context.getSystemService(ClipboardManager::class.java) ?: return
        val clip = ClipData.newPlainText(LABEL, code)
        clip.description.extras = PersistableBundle().apply {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                putBoolean(ClipDescription.EXTRA_IS_SENSITIVE, true)
            } else {
                putBoolean("android.content.extra.IS_SENSITIVE", true)
            }
        }
        manager.setPrimaryClip(clip)
        pending?.let(handler::removeCallbacks)
        if (clearAfterSeconds <= 0) return
        val wipe = Runnable {
            // In the background the clipboard cannot be read: only its label tells it is still ours.
            val current = runCatching { manager.primaryClipDescription?.label?.toString() }.getOrNull()
            val text = runCatching { manager.primaryClip?.getItemAt(0)?.text?.toString() }.getOrNull()
            if (current == LABEL && (text == null || text == code)) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) manager.clearPrimaryClip()
                else manager.setPrimaryClip(ClipData.newPlainText("", ""))
            }
        }
        pending = wipe
        handler.postDelayed(wipe, clearAfterSeconds * 1000L)
    }

    private const val LABEL = "2FAuth"
}

/** The same hue per service as the web panel, so that a service has one colour everywhere. */
fun identityHueOf(service: String?, account: String?, id: Any): Float {
    val source = (service?.takeIf { it.isNotEmpty() } ?: account?.takeIf { it.isNotEmpty() } ?: id.toString()).lowercase()
    var hash = 0L
    for (char in source) hash = (hash * 31 + char.code) and 0xFFFFFFFFL
    return Math.round((hash * 0.61803398875 * 360) % 360).toFloat()
}

fun initialsOf(service: String?, account: String?): String {
    val source = service?.takeIf { it.isNotBlank() } ?: account?.takeIf { it.isNotBlank() } ?: "?"
    val words = source.replace(Regex("[^\\p{L}\\p{N}\\s]"), " ").trim().split(Regex("\\s+")).filter { it.isNotEmpty() }
    if (words.size >= 2) return (words[0].take(1) + words[1].take(1)).uppercase()
    return source.take(2).uppercase()
}
