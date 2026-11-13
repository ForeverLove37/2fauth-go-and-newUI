package app.twofauth.android.data

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** What a sync did, for the message shown to the user. */
data class SyncReport(
    val received: Int = 0,
    val sent: Int = 0,
    val updatedFromServer: Int = 0,
    val detached: Int = 0,
    val removed: Int = 0,
)

/**
 * Brings the synced accounts of the phone and of the server together.
 *
 * - Changed on the web (another revision there): the server's version replaces the
 *   phone's, even if the phone changed it too.
 * - Changed only on the phone: sent to the server.
 * - HOTP counters: both sides keep the highest value.
 * - Made cloud-only on the web: the phone drops its copy of the secret.
 * - Deleted on the web: kept on the phone as a phone-only account, marked as such, so
 *   that no secret disappears without the user deciding it.
 * - Deleted on the phone: deleted on the server, later if the phone is offline.
 */
class SyncEngine(private val api: Api, private val store: LocalStore) {

    suspend fun run(): SyncReport {
        var report = SyncReport()

        val deleted = mutableListOf<Long>()
        for (id in store.pendingDeletions()) {
            try {
                api.delete(id)
                deleted += id
            } catch (error: ApiException) {
                if (error.status == 404 || error.status == 403) deleted += id else throw error
            }
        }
        if (deleted.isNotEmpty()) store.clearDeletions(deleted)

        val feed = api.sync()
        val groups = api.groups().associateBy { it.id }
        val byServerId = feed.synced.associateBy { it.id }
        val cloudIds = feed.cloudIds.toSet()
        val pushed = mutableSetOf<String>()
        val accounts = store.accounts.value

        // first the accounts the phone knows
        val kept = mutableListOf<LocalAccount>()
        for (local in accounts) {
            if (local.kind != Kind.SYNCED) {
                kept += local
                continue
            }
            val serverId = local.serverId
            if (serverId == null) {
                val created = api.create(fields(local, groups) + ("phone_sync" to kotlinx.serialization.json.JsonPrimitive(true)))
                kept += local.copy(serverId = created.id, dirty = false, icon = created.icon ?: local.icon)
                pushed += local.id
                report = report.copy(sent = report.sent + 1)
                continue
            }
            val remote = byServerId[serverId]
            when {
                remote == null && serverId in cloudIds -> report = report.copy(removed = report.removed + 1)
                remote == null -> {
                    kept += local.copy(kind = Kind.LOCAL, serverId = null, revision = null, dirty = false, detached = true)
                    report = report.copy(detached = report.detached + 1)
                }
                remote.revision != local.revision -> {
                    kept += fromServer(remote, groups, local)
                    report = report.copy(updatedFromServer = report.updatedFromServer + 1)
                }
                local.dirty || local.counter > (remote.counter ?: 0) || local.favorite != remote.isFavorite -> {
                    if (local.dirty || local.counter > (remote.counter ?: 0)) api.update(serverId, fields(local, groups))
                    if (local.favorite != remote.isFavorite) api.favorite(serverId, local.favorite)
                    kept += local.copy(dirty = false)
                    pushed += local.id
                    report = report.copy(sent = report.sent + 1)
                }
                else -> kept += local.copy(counter = maxOf(local.counter, remote.counter ?: 0))
            }
        }

        // then the accounts synced from the web that the phone does not have yet
        val known = kept.mapNotNull { it.serverId }.toSet()
        for (remote in feed.synced) {
            if (remote.id !in known && remote.secret != null) {
                kept += fromServer(remote, groups, null)
                report = report.copy(received = report.received + 1)
            }
        }

        // the revisions of what was just sent, as the server now computes them
        val final = if (pushed.isNotEmpty()) {
            val fresh = api.sync().synced.associateBy { it.id }
            kept.map { account -> if (account.id in pushed) account.copy(revision = fresh[account.serverId]?.revision) else account }
        } else {
            kept
        }
        store.update { current ->
            // Accounts added on the phone while the sync ran are kept as they are, and
            // those deleted meanwhile stay deleted.
            val handled = accounts.map { it.id }.toSet()
            val present = current.map { it.id }.toSet()
            final.filter { it.id in present || it.id !in handled } + current.filter { it.id !in handled }
        }
        return report
    }

    /** Makes a phone-only account a synced one: the server receives its secret. */
    suspend fun upload(local: LocalAccount): LocalAccount {
        val groups = api.groups().associateBy { it.id }
        val created = api.create(fields(local, groups) + ("phone_sync" to kotlinx.serialization.json.JsonPrimitive(true)))
        if (local.favorite) runCatching { api.favorite(created.id, true) }
        val revision = api.sync().synced.firstOrNull { it.id == created.id }?.revision
        return local.copy(kind = Kind.SYNCED, serverId = created.id, revision = revision, dirty = false, detached = false, icon = created.icon)
    }

    private suspend fun fields(local: LocalAccount, groups: Map<Long, Group>): JsonObject {
        val groupId = local.group.takeIf { it.isNotBlank() }?.let { name ->
            groups.values.firstOrNull { it.name.equals(name, true) }?.id ?: runCatching { api.createGroup(name).id }.getOrNull()
        }
        return buildJsonObject {
            put("otp_type", local.otpType)
            put("service", local.service)
            put("account", local.account.ifBlank { local.service.ifBlank { "-" } })
            put("secret", local.secret)
            if (!local.isSteam) {
                put("digits", local.digits)
                put("algorithm", local.algorithm)
            }
            if (local.isHotp) put("counter", local.counter) else put("period", local.period)
            put("group_id", groupId ?: 0)
        }
    }

    private fun fromServer(remote: Account, groups: Map<Long, Group>, local: LocalAccount?): LocalAccount {
        val base = local ?: LocalAccount(kind = Kind.SYNCED, secret = remote.secret.orEmpty())
        return base.copy(
            kind = Kind.SYNCED,
            serverId = remote.id,
            service = remote.service.orEmpty(),
            account = remote.account.orEmpty(),
            secret = remote.secret ?: base.secret,
            otpType = remote.otpType,
            digits = remote.digits,
            algorithm = remote.algorithm,
            period = remote.period ?: 30,
            counter = maxOf(remote.counter ?: 0, local?.counter ?: 0),
            group = remote.groupId?.let { groups[it]?.name }.orEmpty(),
            favorite = remote.isFavorite,
            icon = remote.icon,
            revision = remote.revision,
            dirty = false,
            detached = false,
        )
    }
}

private operator fun JsonObject.plus(pair: Pair<String, kotlinx.serialization.json.JsonElement>): JsonObject =
    JsonObject(this.toMutableMap().apply { put(pair.first, pair.second) })
