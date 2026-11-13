package app.twofauth.android.data

import android.content.Context
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request

/**
 * Logos of the services, kept on the phone so that the codes screen never waits for the
 * network. Icons of server accounts come from the server; the others from the same
 * public libraries the server uses (selfh.st, dashboard-icons, Simple Icons), which see
 * the name of the service but nothing else.
 */
class IconCache(context: Context) {
    private val dir = File(context.filesDir, "icons").apply { mkdirs() }
    private val public = OkHttpClient.Builder().connectTimeout(8, TimeUnit.SECONDS).readTimeout(10, TimeUnit.SECONDS).build()
    private val locks = mutableMapOf<String, Mutex>()

    private fun lockFor(key: String) = synchronized(locks) { locks.getOrPut(key) { Mutex() } }

    private fun name(key: String): String =
        MessageDigest.getInstance("SHA-256").digest(key.toByteArray()).joinToString("") { "%02x".format(it) }.take(32)

    /** A cached icon, without touching the network. */
    fun cached(key: String): File? = File(dir, name(key)).takeIf { it.exists() && it.length() > 0 }

    /** The icon a server stores under that name. */
    suspend fun fromServer(api: Api, icon: String): File? = fetch("server:${api.base}:$icon") {
        api.bytes("/storage/icons/" + java.net.URLEncoder.encode(icon, "UTF-8").replace("+", "%20"))
    }

    /** The icon of a service from the public libraries, tried in order. */
    suspend fun forService(service: String): File? {
        val slug = slug(service)
        if (slug.isEmpty()) return null
        return fetch("service:$slug") {
            val candidates = listOf(slug, slug.replace("-", "")).distinct()
            for (candidate in candidates) {
                for (template in TEMPLATES) {
                    val bytes = runCatching { get(template.replace("{slug}", candidate)) }.getOrNull()
                    if (bytes != null) return@fetch bytes
                }
            }
            null
        }
    }

    private fun get(url: String): ByteArray? {
        public.newCall(Request.Builder().url(url).header("User-Agent", "2FAuth-Android").build()).execute().use { response ->
            if (!response.isSuccessful) return null
            val bytes = response.body?.bytes() ?: return null
            return bytes.takeIf { it.size in 40..MAX_SIZE && looksLikeImage(it) }
        }
    }

    /**
     * Downloads once; a miss is remembered for a week so that a service without a logo
     * does not cost a request at every start.
     */
    private suspend fun fetch(key: String, download: suspend () -> ByteArray?): File? = withContext(Dispatchers.IO) {
        lockFor(key).withLock {
            val file = File(dir, name(key))
            if (file.exists() && file.length() > 0) return@withLock file
            val miss = File(dir, name(key) + ".miss")
            if (miss.exists() && System.currentTimeMillis() - miss.lastModified() < TimeUnit.DAYS.toMillis(7)) return@withLock null
            val bytes = runCatching { download() }.getOrNull()
            if (bytes == null || !looksLikeImage(bytes)) {
                miss.writeText("")
                return@withLock null
            }
            val tmp = File(dir, file.name + ".tmp")
            tmp.writeBytes(bytes)
            tmp.renameTo(file)
            miss.delete()
            file
        }
    }

    fun clear() {
        dir.listFiles()?.forEach { it.delete() }
    }

    companion object {
        private const val MAX_SIZE = 512 * 1024
        private val TEMPLATES = listOf(
            "https://cdn.jsdelivr.net/gh/selfhst/icons/svg/{slug}.svg",
            "https://cdn.jsdelivr.net/gh/homarr-labs/dashboard-icons/svg/{slug}.svg",
            "https://cdn.simpleicons.org/{slug}",
        )

        /** The slug rule of the server: lower case, dashes, [a-z0-9.-] only. */
        fun slug(service: String): String = service.trim().lowercase()
            .replace(Regex("[\\s_]+"), "-")
            .replace(Regex("[^a-z0-9.-]"), "")
            .trim('-', '.')

        fun looksLikeImage(bytes: ByteArray): Boolean {
            val head = String(bytes, 0, minOf(bytes.size, 256), Charsets.ISO_8859_1).trimStart()
            return head.startsWith("<svg") || head.startsWith("<?xml") ||
                (bytes.size > 8 && bytes[0] == 0x89.toByte() && bytes[1] == 'P'.code.toByte()) || // PNG
                (bytes.size > 12 && head.startsWith("RIFF") && String(bytes, 8, 4, Charsets.ISO_8859_1) == "WEBP") ||
                (bytes.size > 3 && bytes[0] == 0xFF.toByte() && bytes[1] == 0xD8.toByte()) // JPEG
        }
    }
}
