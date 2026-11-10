package app.twofauth.android

import android.app.Application
import android.os.Build
import androidx.biometric.BiometricManager
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import app.twofauth.android.data.Account
import app.twofauth.android.data.Api
import app.twofauth.android.data.ApiException
import app.twofauth.android.data.FrontendConfig
import app.twofauth.android.data.Group
import app.twofauth.android.data.ImportResult
import app.twofauth.android.data.LocalSettings
import app.twofauth.android.data.OtpPayload
import app.twofauth.android.data.SessionJar
import app.twofauth.android.data.User
import app.twofauth.android.data.Vault
import app.twofauth.android.data.isHostnameMismatch
import app.twofauth.android.data.json
import app.twofauth.android.data.needsRefresh
import app.twofauth.android.data.normalizeServer
import app.twofauth.android.data.peekCertificate
import app.twofauth.android.data.sha256
import app.twofauth.android.data.untrustedCertificate
import app.twofauth.android.ui.LOCK_AUTHENTICATORS
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject
import okhttp3.Cookie
import okhttp3.HttpUrl.Companion.toHttpUrl

/** A certificate the user is asked to trust, with what to do once it is. */
data class CertificatePrompt(val server: String, val fingerprint: String, val subject: String, val hostnameOnly: Boolean)

data class UiState(
    val ready: Boolean = false,
    val server: String = "",
    val config: FrontendConfig? = null,
    val user: User? = null,
    val viaToken: Boolean = true,
    val accounts: List<Account> = emptyList(),
    val groups: List<Group> = emptyList(),
    val loading: Boolean = false,
    val refreshing: Boolean = false,
    val busy: Boolean = false,
    val error: String? = null,
    val message: String? = null,
    val certificate: CertificatePrompt? = null,
    /** Codes of HOTP accounts the user asked for in this session. */
    val hotp: Map<Long, OtpPayload> = emptyMap(),
    val locked: Boolean = false,
)

class AppModel(application: Application) : AndroidViewModel(application) {
    private val vault = Vault(application)
    val settings = LocalSettings(application)
    private val _state = MutableStateFlow(UiState())
    val state: StateFlow<UiState> = _state

    private var api: Api? = null
    private var ticker: Job? = null
    private var lastRefresh = 0L
    private var backgroundSince = 0L

    init {
        viewModelScope.launch { restore() }
    }

    // ------------------------------------------------------------------ session

    private suspend fun restore() {
        val server = vault.get(KEY_SERVER)
        if (server == null) {
            _state.update { it.copy(ready = true, server = settings.lastServer) }
            return
        }
        val pin = vault.get(KEY_PIN)
        val restored = Api(server, pin, vault.get(KEY_TOKEN), SessionJar(SessionJar.parse(server, vault.get(KEY_COOKIES))))
        api = restored
        val cachedUser = vault.get(KEY_USER)?.let { runCatching { json.decodeFromString<User>(it) }.getOrNull() }
        _state.update {
            it.copy(
                ready = true,
                server = server,
                user = cachedUser,
                viaToken = restored.token != null,
                locked = lockEnabled(),
            )
        }
        refresh(initial = true)
    }

    private fun persist(target: Api, user: User?) {
        vault.put(KEY_SERVER, target.base)
        vault.put(KEY_PIN, target.pin)
        vault.put(KEY_TOKEN, target.token)
        vault.put(KEY_COOKIES, if (target.token == null) target.jar.serialize() else null)
        vault.put(KEY_USER, user?.let { json.encodeToString(User.serializer(), it) })
        settings.lastServer = target.base
    }

    /** First step of signing in: is there a 2FAuth server at that address? */
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
                    it.copy(
                        busy = false,
                        certificate = CertificatePrompt(server, certificate.sha256(), certificate.subjectX500Principal.name, untrusted == null),
                    )
                }
                return
            }
        }
        _state.update { it.copy(busy = false, error = describe(error)) }
    }

    fun trustCertificate(prompt: CertificatePrompt) = connect(prompt.server, prompt.fingerprint)

    fun dismissCertificate() = _state.update { it.copy(certificate = null) }

    fun resetServer() {
        api = null
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

    /** The cookies left in the web view by a single sign-on, as "name" to "value". */
    fun signInWithCookies(cookies: Map<String, String>) {
        val target = api ?: return
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            val url = target.base.toHttpUrl()
            val path = url.encodedPath.ifEmpty { "/" }
            target.jar.saveFromResponse(url, cookies.map { (name, value) ->
                Cookie.Builder().name(name).value(value).hostOnlyDomain(url.host).path(path)
                    .apply { if (url.isHttps) secure() }
                    .build()
            })
            try {
                finishSignIn(target)
            } catch (error: Throwable) {
                target.jar.clear()
                _state.update { it.copy(busy = false, error = describe(error)) }
            }
        }
    }

    fun signInWithToken(token: String) {
        val target = api ?: return
        target.token = token.trim()
        _state.update { it.copy(busy = true, error = null) }
        viewModelScope.launch {
            try {
                val user = target.me()
                vault.put(KEY_TOKEN_ID, null)
                persist(target, user)
                _state.update { it.copy(busy = false, user = user, viaToken = true) }
                refresh(initial = true)
            } catch (error: Throwable) {
                target.token = null
                _state.update { it.copy(busy = false, error = describe(error)) }
            }
        }
    }

    /**
     * A session cookie expires after two idle hours: the app swaps it for a token of its
     * own, revocable from the web panel, and closes the session. A server that refuses
     * tokens (single sign-on only) keeps the session.
     */
    private suspend fun finishSignIn(target: Api) {
        val user = target.me()
        val created = runCatching { target.createToken("Android · ${Build.MANUFACTURER} ${Build.MODEL}".take(255)) }.getOrNull()
        if (created != null) {
            target.endSession()
            target.token = created.accessToken
            vault.put(KEY_TOKEN_ID, created.accessTokenId)
        }
        persist(target, user)
        _state.update { it.copy(busy = false, user = user, viaToken = target.token != null) }
        refresh(initial = true)
    }

    fun signOut() {
        val target = api
        viewModelScope.launch {
            if (target != null) {
                val tokenId = vault.get(KEY_TOKEN_ID)
                if (target.token != null && tokenId != null) runCatching { target.deleteToken(tokenId) }
                if (target.token == null) target.endSession()
            }
            clearLocal()
        }
    }

    private fun clearLocal(message: String? = null) {
        val server = api?.base ?: _state.value.server
        api = null
        ticker?.cancel()
        vault.clear()
        settings.lastServer = server
        _state.value = UiState(ready = true, server = server, error = message)
    }

    // ------------------------------------------------------------------ data

    fun iconUrl(icon: String?): String? = api?.iconUrl(icon)

    fun pinOf(): String? = api?.pin

    /** The HTTP client of the server, so that icons go through the same certificate trust. */
    fun http(): okhttp3.OkHttpClient? = api?.client

    fun refresh(initial: Boolean = false, manual: Boolean = false) {
        val target = api ?: return
        if (_state.value.user == null && !initial) return
        lastRefresh = System.currentTimeMillis()
        _state.update { it.copy(loading = initial && it.accounts.isEmpty(), refreshing = manual) }
        viewModelScope.launch {
            try {
                val accounts = target.accounts()
                val groups = runCatching { target.groups() }.getOrDefault(_state.value.groups)
                val user = if (initial || manual) runCatching { target.me() }.getOrNull() else null
                if (user != null) vault.put(KEY_USER, json.encodeToString(User.serializer(), user))
                _state.update {
                    it.copy(
                        accounts = accounts,
                        groups = groups,
                        user = user ?: it.user,
                        loading = false,
                        refreshing = false,
                        error = null,
                    )
                }
                startTicker()
            } catch (error: Throwable) {
                if (error is ApiException && error.isAuth) {
                    clearLocal(getApplication<Application>().getString(R.string.session_expired))
                    return@launch
                }
                _state.update { it.copy(loading = false, refreshing = false, error = describe(error)) }
            }
        }
    }

    /** Fetches new codes as soon as a card has moved on to the last code it knows. */
    private fun startTicker() {
        if (ticker?.isActive == true) return
        ticker = viewModelScope.launch {
            while (isActive) {
                delay(1000)
                if (backgroundSince > 0) continue
                val now = System.currentTimeMillis()
                val due = _state.value.accounts.any { it.needsRefresh(now) }
                if (due && now - lastRefresh > MIN_REFRESH_GAP) refresh()
            }
        }
    }

    fun onForeground() {
        val away = if (backgroundSince > 0) System.currentTimeMillis() - backgroundSince else 0
        backgroundSince = 0
        if (_state.value.user != null && lockEnabled() && away > settings.lockAfter * 1000L) {
            _state.update { it.copy(locked = true) }
        }
        // Codes fetched before the phone slept are stale.
        if (_state.value.user != null && away > 0) refresh()
    }

    fun onBackground() {
        backgroundSince = System.currentTimeMillis()
        // Lock right away when "immediately" is chosen, so the recent apps screen shows the lock.
        if (lockEnabled() && settings.lockAfter == 0 && _state.value.user != null) {
            _state.update { it.copy(locked = true) }
        }
    }

    fun unlock() = _state.update { it.copy(locked = false) }

    fun lockNow() {
        if (lockEnabled()) _state.update { it.copy(locked = true) }
    }

    /** A lock the phone cannot open (no screen lock set) would lock the user out. */
    fun lockEnabled(): Boolean = settings.appLock &&
        BiometricManager.from(getApplication()).canAuthenticate(LOCK_AUTHENTICATORS) == BiometricManager.BIOMETRIC_SUCCESS

    suspend fun hotp(account: Account): OtpPayload? {
        val target = api ?: return null
        return try {
            val payload = target.otp(account.id)
            // The counter moved on the server: the badge of the card follows.
            _state.update { state ->
                state.copy(
                    hotp = state.hotp + (account.id to payload),
                    accounts = state.accounts.map { if (it.id == account.id && payload.counter != null) it.copy(counter = payload.counter) else it },
                )
            }
            payload
        } catch (error: Throwable) {
            report(error)
            null
        }
    }

    fun setFavorite(account: Account, value: Boolean) = mutate {
        it.favorite(account.id, value)
        _state.update { state -> state.copy(accounts = state.accounts.map { a -> if (a.id == account.id) a.copy(isFavorite = value) else a }) }
    }

    fun moveToGroup(account: Account, group: Group?) = mutate {
        if (group == null) it.withdraw(listOf(account.id)) else it.assign(group.id, listOf(account.id))
        refresh()
    }

    fun delete(account: Account) = mutate {
        it.delete(account.id)
        _state.update { state -> state.copy(accounts = state.accounts.filter { a -> a.id != account.id }) }
    }

    fun setServerPreference(key: String, value: Boolean) = mutate {
        it.setPreference(key, value)
        val user = it.me()
        vault.put(KEY_USER, json.encodeToString(User.serializer(), user))
        _state.update { state -> state.copy(user = user) }
    }

    suspend fun decodeQr(jpeg: ByteArray): String? = call { it.decodeQr(jpeg) }

    suspend fun preview(uri: String): Account? = call { it.preview(uri) }

    suspend fun create(fields: JsonObject): Account? = call { it.create(fields) }?.also { refresh() }

    suspend fun import(payload: String): ImportResult? = call { it.import(payload) }?.also { refresh() }

    fun say(message: String) = _state.update { it.copy(message = message) }

    fun consumeMessage() = _state.update { it.copy(message = null) }

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
            clearLocal(getApplication<Application>().getString(R.string.session_expired))
            return
        }
        _state.update { it.copy(message = describe(error)) }
    }

    private fun describe(error: Throwable): String {
        val app = getApplication<Application>()
        return when {
            error is ApiException && error.status == 401 -> app.getString(R.string.error_credentials)
            error is ApiException && error.status == 403 && error.message.contains("forbidden", true) ->
                app.getString(R.string.error_forbidden_ip)
            error is ApiException -> error.message
            error is kotlinx.serialization.SerializationException -> app.getString(R.string.error_not_2fauth)
            error is java.net.UnknownHostException -> app.getString(R.string.error_unknown_host)
            error is java.net.ConnectException || error is java.net.SocketTimeoutException -> app.getString(R.string.error_unreachable)
            error is javax.net.ssl.SSLException -> app.getString(R.string.error_tls, error.message.orEmpty())
            else -> error.message ?: error.javaClass.simpleName
        }
    }

    private companion object {
        const val KEY_SERVER = "server"
        const val KEY_PIN = "pin"
        const val KEY_TOKEN = "token"
        const val KEY_TOKEN_ID = "token_id"
        const val KEY_COOKIES = "cookies"
        const val KEY_USER = "user"
        const val MIN_REFRESH_GAP = 4000L
    }
}
