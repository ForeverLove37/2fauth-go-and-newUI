package app.twofauth.android.data

import android.content.Context
import java.io.File
import java.util.UUID
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.Serializable

/**
 * Where a code lives.
 *
 * - LOCAL: on this phone only. Nothing about it is ever sent to a server.
 * - SYNCED: on the phone and on the server; the server's version wins when both changed.
 * (Cloud codes are not stored here at all: the phone only fetches their codes.)
 */
@Serializable
enum class Kind { LOCAL, SYNCED }

@Serializable
data class LocalAccount(
    val id: String = UUID.randomUUID().toString(),
    val kind: Kind = Kind.LOCAL,
    val service: String = "",
    val account: String = "",
    val secret: String,
    val otpType: String = "totp",
    val digits: Int = 6,
    val algorithm: String = "sha1",
    val period: Int = 30,
    val counter: Long = 0,
    val group: String = "",
    val favorite: Boolean = false,
    /** the icon file name on the server, for synced accounts */
    val icon: String? = null,
    val serverId: Long? = null,
    /** what the server said last time; another value there means it was changed on the web */
    val revision: String? = null,
    /** changed on the phone since the last sync */
    val dirty: Boolean = false,
    /** was synced, then deleted on the server: kept here so that no secret is lost */
    val detached: Boolean = false,
    val createdAt: Long = System.currentTimeMillis(),
) {
    val isHotp: Boolean get() = otpType == "hotp"
    val isSteam: Boolean get() = otpType == "steamtotp"

    fun label(): String = service.ifBlank { account.ifBlank { "?" } }
    fun subLabel(): String = if (service.isNotBlank() && account.isNotBlank()) account else otpType.uppercase()

    /** The code valid at this moment ("" for an HOTP account, which needs a tap). */
    fun codeAt(millis: Long): String =
        if (isHotp) "" else OtpEngine.totpAt(secret, millis / 1000, period, digits, algorithm, isSteam)

    fun remaining(millis: Long): Int = period - ((millis / 1000) % period).toInt()
}

@Serializable
private data class VaultFile(
    val version: Int = 1,
    val accounts: List<LocalAccount> = emptyList(),
    /** server ids of synced accounts deleted on the phone while offline */
    val tombstones: List<Long> = emptyList(),
)

/**
 * The accounts kept on the phone, in one file sealed with a key of the Android Keystore.
 * It is read once at start-up (a few milliseconds), then served from memory.
 */
class LocalStore(context: Context, private val vault: Vault) {
    private val file = File(context.filesDir, "accounts.vault")
    private val lock = Mutex()
    private val _accounts = MutableStateFlow<List<LocalAccount>>(emptyList())
    val accounts: StateFlow<List<LocalAccount>> = _accounts
    private val _loaded = MutableStateFlow(false)
    val loaded: StateFlow<Boolean> = _loaded
    private var tombstones: List<Long> = emptyList()

    suspend fun load() = withContext(Dispatchers.IO) {
        lock.withLock {
            if (file.exists()) {
                val content = runCatching { json.decodeFromString<VaultFile>(String(vault.open(file.readBytes()), Charsets.UTF_8)) }
                    .getOrNull()
                if (content != null) {
                    _accounts.value = content.accounts
                    tombstones = content.tombstones
                } else {
                    // Never overwrite secrets that could not be read: keep the file aside.
                    file.renameTo(File(file.parentFile, "accounts.unreadable-${System.currentTimeMillis()}"))
                }
            }
            _loaded.value = true
        }
    }

    private fun write(accounts: List<LocalAccount>) {
        val sealed = vault.seal(json.encodeToString(VaultFile.serializer(), VaultFile(1, accounts, tombstones)).toByteArray())
        val tmp = File(file.parentFile, file.name + ".tmp")
        tmp.writeBytes(sealed)
        if (!tmp.renameTo(file)) {
            file.delete()
            tmp.renameTo(file)
        }
        _accounts.value = accounts
    }

    /** Changes the list in one step, saved before it is shown. */
    suspend fun update(change: (List<LocalAccount>) -> List<LocalAccount>) = withContext(Dispatchers.IO) {
        lock.withLock { write(change(_accounts.value)) }
    }

    suspend fun add(account: LocalAccount) = update { it + account }

    suspend fun replace(account: LocalAccount) = update { list -> list.map { if (it.id == account.id) account else it } }

    suspend fun remove(id: String) = update { list ->
        val gone = list.firstOrNull { it.id == id }
        if (gone?.kind == Kind.SYNCED && gone.serverId != null) tombstones = tombstones + gone.serverId
        list.filter { it.id != id }
    }

    fun pendingDeletions(): List<Long> = tombstones

    suspend fun clearDeletions(done: Collection<Long>) = withContext(Dispatchers.IO) {
        lock.withLock {
            tombstones = tombstones.filter { it !in done }
            write(_accounts.value)
        }
    }

    fun find(id: String): LocalAccount? = _accounts.value.firstOrNull { it.id == id }

    /** An account with the same secret is already here: adding it again would only duplicate it. */
    fun hasSecret(secret: String): Boolean = _accounts.value.any { it.secret.equals(secret, true) }
}
