package app.twofauth.android.data

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull

val json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    coerceInputValues = true
    isLenient = true
}

@Serializable
data class OtpPayload(
    val password: String = "",
    @SerialName("otp_type") val otpType: String? = null,
    @SerialName("generated_at") val generatedAt: Long? = null,
    @SerialName("next_password") val nextPassword: String? = null,
    val period: Int? = null,
    val counter: Long? = null,
)

@Serializable
data class Account(
    /** 0 for a preview, which is not saved yet. */
    val id: Long = 0,
    @SerialName("group_id") val groupId: Long? = null,
    @SerialName("otp_type") val otpType: String = "totp",
    val account: String? = null,
    val service: String? = null,
    val icon: String? = null,
    val digits: Int = 6,
    val algorithm: String = "sha1",
    val period: Int? = null,
    val counter: Long? = null,
    @SerialName("is_favorite") val isFavorite: Boolean = false,
    @SerialName("is_borrowed") val isBorrowed: Boolean = false,
    @SerialName("borrowed_by") val borrowedBy: String? = null,
    @SerialName("is_shared") val isShared: Boolean = false,
    @SerialName("is_shared_with_all") val isSharedWithAll: Boolean = false,
    val otp: OtpPayload? = null,
) {
    val isHotp: Boolean get() = otpType == "hotp"
}

@Serializable
data class Group(
    val id: Long,
    val name: String,
    @SerialName("show_in_chips") val showInChips: Boolean = true,
    @SerialName("twofaccounts_count") val count: Int = 0,
)

@Serializable
data class User(
    val id: Long,
    val name: String,
    val email: String,
    @SerialName("is_admin") val isAdmin: Boolean = false,
    val preferences: JsonObject = JsonObject(emptyMap()),
) {
    fun preference(key: String, fallback: Boolean): Boolean =
        (preferences[key] as? JsonPrimitive)?.booleanOrNull ?: fallback
}

@Serializable
data class SsoFlags(val openid: Boolean = false)

@Serializable
data class AppSettings(
    val disableRegistration: Boolean = false,
    val useSsoOnly: Boolean = false,
    val allowPatWhileSsoOnly: Boolean = false,
)

@Serializable
data class FrontendConfig(
    val appName: String = "2FAuth",
    val appVersion: String = "",
    val sso: SsoFlags = SsoFlags(),
    val appSettings: AppSettings = AppSettings(),
)

@Serializable
data class Envelope<T>(val data: T)

@Serializable
data class CreatedToken(val accessToken: String, val accessTokenId: String)

@Serializable
data class ImportResult(
    val total: Int = 0,
    val imported: Int = 0,
    val duplicates: Int = 0,
    val invalid: Int = 0,
)

/** What a request failed with, in the words of the server when it gave some. */
class ApiException(
    val status: Int,
    override val message: String,
    val errors: Map<String, List<String>> = emptyMap(),
) : Exception(message) {
    val isAuth: Boolean get() = status == 401 || status == 418

    companion object {
        fun from(status: Int, body: String): ApiException {
            val parsed = runCatching { json.parseToJsonElement(body) as? JsonObject }.getOrNull()
            val message = (parsed?.get("message") as? JsonPrimitive)?.content
            val errors = (parsed?.get("errors") as? JsonObject)?.mapValues { (_, value: JsonElement) ->
                (value as? JsonArray)?.mapNotNull { (it as? JsonPrimitive)?.content } ?: emptyList()
            } ?: emptyMap()
            val first = errors.values.flatten().firstOrNull()
            return ApiException(status, first ?: message ?: "HTTP $status", errors)
        }
    }
}
