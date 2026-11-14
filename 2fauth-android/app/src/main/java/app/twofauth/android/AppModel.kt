package app.twofauth.android

import android.app.Application
import android.os.Build
import androidx.biometric.BiometricManager
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import app.twofauth.android.data.Account
import app.twofauth.android.ui.Copied
import app.twofauth.android.data.AndroidRelease
import app.twofauth.android.data.Api
import app.twofauth.android.data.ApiException
import app.twofauth.android.data.FrontendConfig
import app.twofauth.android.data.Group
import app.twofauth.android.data.ImportResult
import app.twofauth.android.data.Kind
import app.twofauth.android.data.LocalAccount
import app.twofauth.android.data.OtpAuth
import app.twofauth.android.data.OtpEngine
import app.twofauth.android.data.OtpPayload
import app.twofauth.android.data.ParsedAccount
import app.twofauth.android.data.SessionJar
import app.twofauth.android.data.SyncEngine
import app.twofauth.android.data.Updates
import app.twofauth.android.data.User
import app.twofauth.android.data.isHostnameMismatch
import app.twofauth.android.data.json
import app.twofauth.android.data.needsRefresh
import app.twofauth.android.data.normalizeServer
import app.twofauth.android.data.peekCertificate
import app.twofauth.android.data.sha256
import app.twofauth.android.data.untrustedCertificate
import app.twofauth.android.ui.LOCK_AUTHENTICATORS
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl

/** A certificate the user is asked to trust. */
data class CertificatePrompt(val server: String, val fingerprint: String, val subject: String, val hostnameOnly: Boolean)

data class UiState(
    val ready: Boolean = false,
    val server: String = "",
    val config: FrontendConfig? = null,
    val user: User? = null,
    val viaToken: Boolean = true,
    /** cloud-only codes: their secret stays on the server */
    val cloud: List<Account> = emptyList(),
    val groups: List<Group> = emptyList(),
    val cloudLoading: Boolean = false,
    val cloudRefreshing: Boolean = false,
    val cloudError: String? = null,
    val busy: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    /** the last code copied, for the toast that confirms it */
    val copied: Copied? = null,
    val certificate: CertificatePrompt? = null,
    val hotp: Map<Long, OtpPayload> = emptyMap(),
    val locked: Boolean = false,
    val syncing: Boolean = false,
    val lastSync: Long = 0,
    val syncError: String? = null,
    val update: AndroidRelease? = null,
    val downloading: Float? = null,
)

class AppModel(application: Application) : AndroidViewModel(application) {
    private val app = application as TwoFAuthApp
    private val vault = app.vault
    val settings = app.settings
    val store = app.store
    val icons = app.icons
    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    private var api: Api? = null
    private var ticker: Job? = null
    private var lastCloudRefresh = 0L
    private var backgroundSince = 0L
    private val syncLock = Mutex()
    private var pendingSync: Job? = null

    init {
        viewModelScope.launch { restore() }
        viewModelScope.launch {
            // While the app is open, a signed-in phone syncs every few minutes.
            while (isActive) {
                delay(SYNC_INTERVAL)
                if (backgroundSince == 0L && api != null) sync()
            }
        }
        if (settings.checkUpdates) viewModelScope.launch { checkUpdate(manual = false) }
    }

    private fun string(id: Int, vararg args: Any): String = getApplication<Application>().getString(id, *args)

    // ------------------------------------------------------------------ session

    private suspend fun restore() {
        if (!store.loaded.value) store.load()
        val server = vault.get(KEY_SERVER)
        if (server == null) {
            _state.update { it.copy(ready = true, server = settings.lastServer, locked = lockEnabled() && store.accounts.value.isNotEmpty()) }
            return
        }
        val restored = Api(server, vault.get(KEY_PIN), vault.get(KEY_TOKEN), SessionJar(SessionJar.parse(server, vault.get(KEY_COOKIES))))
        api = restored
        val cachedUser = vault.get(KEY_USER)?.let { runCatching { json.decodeFromString<User>(it) }.getOrNull() }
        _state.update { it.copy(ready = true, server = server, user = cachedUser, viaToken = restored.token != null, locked = lockEnabled()) }
        viewModelScope.launch {
            upgradeToken(restored)
            sync()
            refreshCloud(initial = true)
        }
    }

    /** Version 1.0 signed in with a full token: swap it for a device token once. */
    private suspend fun upgradeToken(target: Api) {
        if (target.token == null || vault.get(KEY_TOKEN_KIND) == "device") return
        val created = runCatching { target.createDeviceToken(deviceName()) }.getOrNull() ?: return
        val oldId = vault.get(KEY_TOKEN_ID)
        val old = target.token
        target.token = created.accessToken
        if (oldId != null && old != null) runCatching { Api(target.base, target.pin, old).deleteToken(oldId) }
        vault.put(KEY_TOKEN, created.accessToken)
        vault.put(KEY_TOKEN_ID, created.accessTokenId)
        vault.put(KEY_TOKEN_KIND, "device")
    }

    private fun deviceName() = "Android · ${Build.MANUFACTURER} ${Build.MODEL}".take(255)

    private fun persist(target: Api, user: User?) {
        vault.put(KEY_SERVER, target.base)
        vault.put(KEY_PIN, target.pin)
        vault.put(KEY_TOKEN, target.token)
        vault.put(KEY_COOKIES, if (target.token == null) target.jar.serialize() else null)
        vault.put(KEY_USER, user?.let { json.encodeToString(User.serializer(), it) })
        settings.lastServer = target.base
    }

    fun connect(input: String, pin: String? = null) {
        val server = normalizeServer(input)
        _state.update { it.copy(busy = true, error = null, server = server) }
        viewModelScope.launch {
            val candidate = Api(server, pin)
            try {
                val config = candidate.config()
                api = candidate
                _state.update { it.copy(busy = false, config = config, certificate = null) }
            } catch (error: Throwable) {
                handleConnectionError(server, error)
            }
        }
    }

    private suspend fun handleConnectionError(server: String, error: Throwable) {
        val untrusted = error.untrustedCertificate()
        val mismatch = error.isHostnameMismatch()
        if (server.startsWith("https://") && (untrusted != null || mismatch)) {
            val certificate = untrusted ?: withContext(Dispatchers.IO) { peekCertificate(server) }
            if (certificate != null) {
                _state.update {
                    it.copy(busy = false, certificate = CertificatePrompt(server, certificate.sha256(), certificate.subjectX500Principal.name, untrusted == null))
                }
                return
            }
        }
        _state.update { it.copy(busy = false, error = describe(error)) }
    }

    fun trustCertificate(prompt: CertificatePrompt) = connect(prompt.server, prompt.fingerprint)

    fun dismissCertificate() = _state.update { it.copy(certificate = null) }

    fun resetServer() {
        if (_state.value.user == null) api = null
        _state.update { it.copy(config = null, error = null) }
    }

    fun signInWithPassword(email: String, password: String) {
        val target = api ?: return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            try {
                target.login(email.trim(), password)
                finishSignIn(target)
            } catch (error: Throwable) {
                target.jar.clear()
                _state.update { it.copy(busy = false, error = describe(error)) }
            }
        }
    }

    fun signInWithCookies(cookies: Map<String, String>) {
        val target = api ?: return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val url = target.base.toHttpUrl()
            val path = url.encodedPath.ifEmpty { "/" }
            target.jar.saveFromResponse(url, cookies.map { (name, value) ->
                Cookie.Builder().name(name).value(value).hostOnlyDomain(url.host).path(path).apply { if (url.isHttps) secure() }.build()
            })
            try {
                finishSignIn(target)
            } catch (error: Throwable) {
                target.jar.clear()
                _state.update { it.copy(busy = false, error = describe(error)) }
            }
        }
    }

    /** A pasted token is only used to obtain the token of this phone; it stays valid. */
    fun signInWithToken(token: String) {
        val target = api ?: return
        target.token = token.trim()
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            try {
                target.me()
                val created = runCatching { target.createDeviceToken(deviceName()) }.getOrNull()
                if (created != null) {
                    target.token = created.accessToken
                    vault.put(KEY_TOKEN_ID, created.accessTokenId)
                    vault.put(KEY_TOKEN_KIND, "device")
                } else {
                    vault.put(KEY_TOKEN_ID, null)
                    vault.put(KEY_TOKEN_KIND, null)
                }
                completeSignIn(target)
            } catch (error: Throwable) {
                target.token = null
                _state.update { it.copy(busy = false, error = describe(error)) }
            }
        }
    }

    /**
     * The session cookie of a sign-in is swapped for a device token: it does not expire
     * after two idle hours, the user can revoke it on the web, and the server refuses it
     * anything a phone does not need. Servers that refuse tokens keep the session.
     */
    private suspend fun finishSignIn(target: Api) {
        target.me()
        val created = runCatching { target.createDeviceToken(deviceName()) }.getOrNull()
        if (created != null) {
            target.endSession()
            target.token = created.accessToken
            vault.put(KEY_TOKEN_ID, created.accessTokenId)
            vault.put(KEY_TOKEN_KIND, "device")
        }
        completeSignIn(target)
    }

    private suspend fun completeSignIn(target: Api) {
        val user = target.me()
        // Synced copies that belong to another server become phone-only ones.
        val previous = vault.get(KEY_SYNCED_SERVER)
        if (previous != null && previous != target.base) {
            store.update { list -> list.map { if (it.kind == Kind.SYNCED) it.copy(kind = Kind.LOCAL, serverId = null, revision = null) else it } }
        }
        vault.put(KEY_SYNCED_SERVER, target.base)
        persist(target, user)
        _state.update { it.copy(busy = false, user = user, viaToken = target.token != null, server = target.base) }
        say(string(R.string.signed_in_as, user.name))
        sync()
        refreshCloud(initial = true)
        if (settings.checkUpdates) checkUpdate(manual = false)
    }

    /**
     * Signing out keeps the codes of the phone. Synced copies become phone-only ones when
     * [keepSynced], and are removed otherwise (they stay on the server).
     */
    fun signOut(keepSynced: Boolean) {
        val target = api
        viewModelScope.launch {
            if (target != null) {
                val tokenId = vault.get(KEY_TOKEN_ID)
                if (target.token != null && tokenId != null) runCatching { target.deleteToken(tokenId) }
                if (target.token == null) target.endSession()
            }
            store.update { list ->
                if (keepSynced) list.map { if (it.kind == Kind.SYNCED) it.copy(kind = Kind.LOCAL, serverId = null, revision = null, dirty = false) else it }
                else list.filter { it.kind != Kind.SYNCED }
            }
            clearSession()
        }
    }

    private fun clearSession(message: String? = null) {
        val server = api?.base ?: _state.value.server
        api = null
        ticker?.cancel()
        listOf(KEY_SERVER, KEY_PIN, KEY_TOKEN, KEY_TOKEN_ID, KEY_TOKEN_KIND, KEY_COOKIES, KEY_USER).forEach { vault.put(it, null) }
        settings.lastServer = server
        _state.update { UiState(ready = true, server = server, message = message, update = it.update) }
    }

    val signedIn: Boolean get() = api != null && _state.value.user != null

    // ------------------------------------------------------------------ codes on the phone

    /** Saves accounts read from a QR code or a link. Returns how many were new. */
    suspend fun addParsed(parsed: List<ParsedAccount>, kind: Kind, group: String = ""): Int {
        val fresh = parsed.filter { !store.hasSecret(it.secret) }
        for (item in fresh) {
            store.add(
                LocalAccount(
                    kind = if (kind == Kind.SYNCED && signedIn) Kind.SYNCED else Kind.LOCAL,
                    service = item.service, account = item.account, secret = item.secret.uppercase(),
                    otpType = item.otpType, digits = item.digits, algorithm = item.algorithm,
                    period = item.period, counter = item.counter, group = group, dirty = kind == Kind.SYNCED,
                ),
            )
        }
        if (fresh.isNotEmpty() && kind == Kind.SYNCED) scheduleSync()
        return fresh.size
    }

    /** Sends accounts to the server as cloud-only codes: the phone keeps nothing. */
    suspend fun addToCloud(parsed: List<ParsedAccount>): Int {
        val target = api ?: return 0
        var count = 0
        for (item in parsed) {
            val created = call {
                target.create(buildJsonObject {
                    put("otp_type", item.otpType)
                    put("service", item.service)
                    put("account", item.account.ifBlank { item.service.ifBlank { "-" } })
                    put("secret", item.secret)
                    if (item.otpType != "steamtotp") {
                        put("digits", item.digits)
                        put("algorithm", item.algorithm)
                    }
                    if (item.otpType == "hotp") put("counter", item.counter) else put("period", item.period)
                })
            }
            if (created != null) count++
        }
        refreshCloud()
        return count
    }

    fun edit(account: LocalAccount) {
        viewModelScope.launch {
            store.replace(if (account.kind == Kind.SYNCED) account.copy(dirty = true) else account)
            if (account.kind == Kind.SYNCED) scheduleSync()
        }
    }

    fun toggleFavorite(account: LocalAccount) = edit(account.copy(favorite = !account.favorite))

    /** Deleting a synced code deletes it on the server too (at the next sync if offline). */
    fun delete(account: LocalAccount) {
        viewModelScope.launch {
            store.remove(account.id)
            if (account.kind == Kind.SYNCED) scheduleSync()
        }
    }

    /** The next code of an HOTP account: the counter moves forward on the phone. */
    suspend fun nextHotp(account: LocalAccount): String {
        val code = runCatching { OtpEngine.hotp(account.secret, account.counter, account.digits, account.algorithm) }.getOrDefault("")
        if (code.isNotEmpty()) edit(account.copy(counter = account.counter + 1))
        return code
    }

    /** Phone-only → synced. The caller has verified the user (fingerprint, screen lock). */
    fun makeSynced(account: LocalAccount) {
        val target = api ?: return say(string(R.string.need_server))
        viewModelScope.launch {
            _state.update { it.copy(syncing = true) }
            try {
                val synced = SyncEngine(target, store).upload(account)
                store.replace(synced)
                vault.put(KEY_SYNCED_SERVER, target.base)
                say(string(R.string.now_synced, account.label()))
            } catch (error: Throwable) {
                report(error)
            } finally {
                _state.update { it.copy(syncing = false) }
            }
        }
    }

    /** Synced → phone-only: the server deletes its copy. */
    fun makeLocal(account: LocalAccount) {
        val target = api ?: return
        val serverId = account.serverId ?: return
        viewModelScope.launch {
            try {
                target.delete(serverId)
                store.replace(account.copy(kind = Kind.LOCAL, serverId = null, revision = null, dirty = false, detached = false))
                say(string(R.string.now_local, account.label()))
            } catch (error: Throwable) {
                report(error)
            }
        }
    }

    /** Synced → cloud-only: the server keeps it, the phone forgets the secret. */
    fun makeCloud(account: LocalAccount) {
        val target = api ?: return
        val serverId = account.serverId ?: return
        viewModelScope.launch {
            try {
                target.setPhoneSync(serverId, false)
                store.update { list -> list.filter { it.id != account.id } }
                say(string(R.string.now_cloud, account.label()))
                refreshCloud()
            } catch (error: Throwable) {
                report(error)
            }
        }
    }

    /** A detached copy stays on the phone: the flag only told the user. */
    fun acknowledgeDetached(account: LocalAccount) = edit(account.copy(detached = false))

    private fun scheduleSync() {
        pendingSync?.cancel()
        pendingSync = viewModelScope.launch {
            delay(1500)
            sync()
        }
    }

    fun sync(manual: Boolean = false) {
        val target = api ?: return
        viewModelScope.launch {
            if (!syncLock.tryLock()) return@launch
            try {
                _state.update { it.copy(syncing = true) }
                val result = SyncEngine(target, store).run()
                _state.update { it.copy(syncing = false, lastSync = System.currentTimeMillis(), syncError = null) }
                val changes = result.received + result.updatedFromServer + result.detached + result.removed
                when {
                    result.detached > 0 -> say(string(R.string.sync_detached, result.detached))
                    manual && changes + result.sent == 0 -> say(string(R.string.sync_nothing))
                    manual || changes > 0 -> say(string(R.string.sync_done, result.received + result.updatedFromServer, result.sent))
                }
            } catch (error: Throwable) {
                if (error is ApiException && error.isAuth) {
                    clearSession(string(R.string.session_expired))
                } else {
                    _state.update { it.copy(syncing = false, syncError = describe(error)) }
                    if (manual) say(describe(error))
                }
            } finally {
                syncLock.unlock()
            }
        }
    }

    // ------------------------------------------------------------------ cloud codes

    fun refreshCloud(initial: Boolean = false, manual: Boolean = false) {
        val target = api ?: return
        lastCloudRefresh = System.currentTimeMillis()
        _state.update { it.copy(cloudLoading = initial && it.cloud.isEmpty(), cloudRefreshing = manual) }
        viewModelScope.launch {
            try {
                val accounts = target.accounts().filter { !it.phoneSync || it.isBorrowed }
                val groups = runCatching { target.groups() }.getOrDefault(_state.value.groups)
                _state.update { it.copy(cloud = accounts, groups = groups, cloudLoading = false, cloudRefreshing = false, cloudError = null) }
                startTicker()
            } catch (error: Throwable) {
                if (error is ApiException && error.isAuth) {
                    clearSession(string(R.string.session_expired))
                    return@launch
                }
                _state.update { it.copy(cloudLoading = false, cloudRefreshing = false, cloudError = describe(error)) }
            }
        }
    }

    private fun startTicker() {
        if (ticker?.isActive == true) return
        ticker = viewModelScope.launch {
            while (isActive) {
                delay(1000)
                if (backgroundSince > 0) continue
                val now = System.currentTimeMillis()
                if (_state.value.cloud.any { it.needsRefresh(now) } && now - lastCloudRefresh > MIN_REFRESH_GAP) refreshCloud()
            }
        }
    }

    suspend fun cloudHotp(account: Account): OtpPayload? {
        val target = api ?: return null
        return try {
            val payload = target.otp(account.id)
            _state.update { state ->
                state.copy(
                    hotp = state.hotp + (account.id to payload),
                    cloud = state.cloud.map { if (it.id == account.id && payload.counter != null) it.copy(counter = payload.counter) else it },
                )
            }
            payload
        } catch (error: Throwable) {
            report(error)
            null
        }
    }

    fun setCloudFavorite(account: Account, value: Boolean) = mutate {
        it.favorite(account.id, value)
        _state.update { s -> s.copy(cloud = s.cloud.map { a -> if (a.id == account.id) a.copy(isFavorite = value) else a }) }
    }

    fun deleteCloud(account: Account) = mutate {
        it.delete(account.id)
        _state.update { s -> s.copy(cloud = s.cloud.filter { a -> a.id != account.id }) }
    }

    suspend fun importToServer(payload: String): ImportResult? = call { it.import(payload) }?.also { refreshCloud() }

    // ------------------------------------------------------------------ icons

    /** Icons already resolved in this process, so that a scrolled card shows its logo at once. */
    val iconMemo = java.util.concurrent.ConcurrentHashMap<Any, File>()

    /** A cached file for an account's logo, fetched on first use. */
    suspend fun iconFile(local: LocalAccount): File? {
        val target = api
        if (local.icon != null && target != null && local.kind == Kind.SYNCED) {
            icons.fromServer(target, local.icon)?.let { return it }
        }
        if (!settings.publicIcons || local.service.isBlank()) return null
        return icons.forService(local.service)
    }

    suspend fun iconFile(cloud: Account): File? {
        val target = api ?: return null
        val name = cloud.icon ?: return if (settings.publicIcons && !cloud.service.isNullOrBlank()) icons.forService(cloud.service) else null
        return icons.fromServer(target, name)
    }

    // ------------------------------------------------------------------ lock and lifecycle

    fun onForeground() {
        val away = if (backgroundSince > 0) System.currentTimeMillis() - backgroundSince else 0
        backgroundSince = 0
        if (lockEnabled() && away > settings.lockAfter * 1000L) _state.update { it.copy(locked = true) }
        if (api != null && away > 0) {
            sync()
            refreshCloud()
        }
    }

    fun onBackground() {
        backgroundSince = System.currentTimeMillis()
        if (lockEnabled() && settings.lockAfter == 0) _state.update { it.copy(locked = true) }
    }

    fun unlock() = _state.update { it.copy(locked = false) }

    fun lockNow() {
        if (lockEnabled()) _state.update { it.copy(locked = true) }
    }

    fun canVerifyUser(): Boolean =
        BiometricManager.from(getApplication()).canAuthenticate(LOCK_AUTHENTICATORS) == BiometricManager.BIOMETRIC_SUCCESS

    fun lockEnabled(): Boolean = settings.appLock && canVerifyUser()

    // ------------------------------------------------------------------ updates

    fun checkUpdate(manual: Boolean) {
        viewModelScope.launch {
            val release = Updates.latest(getApplication())
            _state.update { it.copy(update = release) }
            if (manual && release == null) say(string(R.string.update_none, BuildConfig.VERSION_NAME))
        }
    }

    fun installUpdate() {
        val release = _state.value.update ?: return
        if (_state.value.downloading != null) return
        viewModelScope.launch {
            _state.update { it.copy(downloading = 0f) }
            try {
                val file = Updates.download(getApplication(), release) { progress -> _state.update { it.copy(downloading = progress) } }
                _state.update { it.copy(downloading = null) }
                if (!Updates.install(getApplication(), file)) say(string(R.string.update_allow))
            } catch (error: Throwable) {
                _state.update { it.copy(downloading = null) }
                say(describe(error))
            }
        }
    }

    fun dismissUpdate() = _state.update { it.copy(update = null) }

    // ------------------------------------------------------------------ messages

    fun say(message: String) = _state.update { it.copy(message = message) }

    fun consumeMessage() = _state.update { it.copy(message = null) }

    private var copies = 0L

    /** Shows the toast that confirms a copy: the service, and how long the code still lives when that is short. */
    fun copied(service: String, left: Int?) {
        val ctx = getApplication<Application>()
        val expiring = left != null && left in 1..5
        val detail = if (expiring) ctx.getString(R.string.copied_expiring, left) else ctx.getString(R.string.copied)
        _state.update { it.copy(copied = Copied(++copies, service, detail, warn = expiring)) }
    }

    fun clearError() = _state.update { it.copy(error = null) }

    private suspend fun <T> call(block: suspend (Api) -> T): T? {
        val target = api ?: return null
        return try {
            block(target)
        } catch (error: Throwable) {
            report(error)
            null
        }
    }

    private fun mutate(block: suspend (Api) -> Unit) {
        viewModelScope.launch { call(block) }
    }

    private fun report(error: Throwable) {
        if (error is ApiException && error.isAuth) {
            clearSession(string(R.string.session_expired))
            return
        }
        say(describe(error))
    }

    fun describe(error: Throwable): String = when {
        error is ApiException && error.status == 401 -> string(R.string.error_credentials)
        error is ApiException && error.status == 403 && error.message.contains("forbidden", true) -> string(R.string.error_forbidden_ip)
        error is ApiException -> error.message
        error is kotlinx.serialization.SerializationException -> string(R.string.error_not_2fauth)
        error is java.net.UnknownHostException -> string(R.string.error_unknown_host)
        error is java.net.ConnectException || error is java.net.SocketTimeoutException -> string(R.string.error_unreachable)
        error is javax.net.ssl.SSLException -> string(R.string.error_tls, error.message.orEmpty())
        else -> error.message ?: error.javaClass.simpleName
    }

    fun http(): okhttp3.OkHttpClient? = api?.client

    fun pinOf(): String? = api?.pin

    private companion object {
        const val KEY_SERVER = "server"
        const val KEY_PIN = "pin"
        const val KEY_TOKEN = "token"
        const val KEY_TOKEN_ID = "token_id"
        const val KEY_TOKEN_KIND = "token_kind"
        const val KEY_COOKIES = "cookies"
        const val KEY_USER = "user"
        const val KEY_SYNCED_SERVER = "synced_server"
        const val MIN_REFRESH_GAP = 4000L
        const val SYNC_INTERVAL = 3 * 60 * 1000L
    }
}

/** The link of a local account for the clipboard of another authenticator. */
fun LocalAccount.otpauth(): String = OtpAuth.uri(this)
