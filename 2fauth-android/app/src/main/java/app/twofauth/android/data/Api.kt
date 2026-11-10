package app.twofauth.android.data

import android.os.Build
import app.twofauth.android.BuildConfig
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import okhttp3.Cookie
import okhttp3.CookieJar
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.MultipartBody
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody

/** "2fa.example.com/" → "https://2fa.example.com", keeping a sub-directory. */
fun normalizeServer(input: String): String {
    var text = input.trim().trimEnd('/')
    if (!text.contains("://")) text = "https://$text"
    return text
}

/** Cookies of the browser-like session, for servers that only allow signing in that way. */
class SessionJar(initial: List<Cookie> = emptyList()) : CookieJar {
    private val cookies = initial.toMutableList()

    @Synchronized
    override fun saveFromResponse(url: HttpUrl, cookies: List<Cookie>) {
        for (cookie in cookies) {
            this.cookies.removeAll { it.name == cookie.name && it.domain == cookie.domain && it.path == cookie.path }
            if (cookie.expiresAt > System.currentTimeMillis()) this.cookies += cookie
        }
    }

    @Synchronized
    override fun loadForRequest(url: HttpUrl): List<Cookie> {
        val now = System.currentTimeMillis()
        cookies.removeAll { it.expiresAt <= now }
        return cookies.filter { it.matches(url) }
    }

    @Synchronized
    fun value(name: String): String? = cookies.firstOrNull { it.name == name }?.value

    @Synchronized
    fun all(): List<Cookie> = cookies.toList()

    @Synchronized
    fun isEmpty(): Boolean = cookies.isEmpty()

    @Synchronized
    fun clear() = cookies.clear()

    fun serialize(): String = all().joinToString("\n")

    companion object {
        fun parse(base: String, text: String?): List<Cookie> {
            if (text.isNullOrBlank()) return emptyList()
            val url = base.toHttpUrl()
            return text.lines().mapNotNull { Cookie.parse(url, it) }
        }
    }
}

/**
 * The JSON API of a 2FAuth server, signed in with a personal access token, or with
 * the session cookies when the server does not hand out tokens.
 */
class Api(val base: String, val pin: String?, var token: String? = null, val jar: SessionJar = SessionJar()) {
    val client: OkHttpClient = PinningTrust(pin)
        .apply(OkHttpClient.Builder())
        .cookieJar(jar)
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(25, TimeUnit.SECONDS)
        .followRedirects(false)
        .build()

    private val userAgent = "2FAuth-Android/${BuildConfig.VERSION_NAME} (Android ${Build.VERSION.RELEASE}; ${Build.MODEL})"

    fun iconUrl(icon: String?): String? =
        icon?.let { "$base/storage/icons/${java.net.URLEncoder.encode(it, "UTF-8").replace("+", "%20")}" }

    suspend fun send(method: String, path: String, body: JsonElement? = null, form: RequestBody? = null, retry: Boolean = true): String =
        withContext(Dispatchers.IO) {
            val payload = form ?: body?.toString()?.toRequestBody(JSON)
                ?: if (method == "GET" || method == "HEAD") null else ByteArray(0).toRequestBody(null)
            val request = Request.Builder()
                .url(base + path)
                .method(method, payload)
                .header("Accept", "application/json")
                .header("User-Agent", userAgent)
                .apply {
                    token?.let { header("Authorization", "Bearer $it") }
                    if (method != "GET" && method != "HEAD") {
                        jar.value("XSRF-TOKEN")?.let { header("X-XSRF-TOKEN", java.net.URLDecoder.decode(it, "UTF-8")) }
                    }
                }
                .build()
            client.newCall(request).execute().use { response ->
                val text = response.body?.string().orEmpty()
                // The CSRF token rotates with the session: fetch a fresh one once and replay.
                if (response.code == 419 && retry) {
                    send("GET", "/refresh-csrf", retry = false)
                    return@withContext send(method, path, body, form, retry = false)
                }
                if (!response.isSuccessful) throw ApiException.from(response.code, text)
                text
            }
        }

    private suspend inline fun <reified T> get(path: String): T = json.decodeFromString(send("GET", path))
    private suspend inline fun <reified T> post(path: String, body: JsonElement? = null): T = json.decodeFromString(send("POST", path, body))

    suspend fun config(): FrontendConfig = get("/api/v1/frontend-config")

    suspend fun login(email: String, password: String) {
        send("GET", "/refresh-csrf")
        send("POST", "/user/login", buildJsonObject {
            put("email", email)
            put("password", password)
        })
    }

    suspend fun createToken(name: String): CreatedToken =
        post("/oauth/personal-access-tokens", buildJsonObject { put("name", name) })

    suspend fun deleteToken(id: String) {
        send("DELETE", "/oauth/personal-access-tokens/$id")
    }

    /** Ends the browser-like session, once a token has replaced it or on sign out. */
    suspend fun endSession() {
        runCatching { send("POST", "/user/logout") }
        jar.clear()
    }

    suspend fun me(): User = get<Envelope<User>>("/api/v1/user").data

    suspend fun accounts(): List<Account> = get<Envelope<List<Account>>>("/api/v1/twofaccounts?withOtp=1").data

    suspend fun groups(): List<Group> = get<Envelope<List<Group>>>("/api/v1/groups").data.filter { it.id > 0 }

    suspend fun otp(id: Long): OtpPayload = get("/api/v1/twofaccounts/$id/otp")

    suspend fun favorite(id: Long, value: Boolean) {
        send("PATCH", "/api/v1/twofaccounts/$id/favorite", buildJsonObject { put("is_favorite", value) })
    }

    suspend fun assign(groupId: Long, ids: List<Long>) {
        send("POST", "/api/v1/groups/$groupId/assign", buildJsonObject { putJsonArray("ids") { ids.forEach { add(kotlinx.serialization.json.JsonPrimitive(it)) } } })
    }

    suspend fun withdraw(ids: List<Long>) {
        send("PATCH", "/api/v1/twofaccounts/withdraw?ids=${ids.joinToString(",")}")
    }

    suspend fun delete(id: Long) {
        send("DELETE", "/api/v1/twofaccounts/$id")
    }

    suspend fun setPreference(key: String, value: Boolean) {
        send("PUT", "/api/v1/user/preferences/$key", buildJsonObject { put("value", value) })
    }

    /** The text of the QR code in a picture, read by the server. */
    suspend fun decodeQr(jpeg: ByteArray): String {
        val form = MultipartBody.Builder().setType(MultipartBody.FORM)
            .addFormDataPart("qrcode", "scan.jpg", jpeg.toRequestBody("image/jpeg".toMediaType()))
            .build()
        val text = send("POST", "/api/v1/qrcode/decode", form = form)
        return json.decodeFromString<Envelope<String>>(text).data
    }

    suspend fun preview(uri: String): Account = post<Envelope<Account>>("/api/v1/twofaccounts/preview", buildJsonObject { put("uri", uri) }).data

    suspend fun create(fields: JsonObject): Account = post<Envelope<Account>>("/api/v1/twofaccounts", fields).data

    /** Everything that is not a single otpauth:// link: Google Authenticator exports, Aegis, 2FAS… */
    suspend fun import(payload: String): ImportResult = post("/api/v1/twofaccounts/import", buildJsonObject { put("payload", payload) })

    private companion object {
        val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
