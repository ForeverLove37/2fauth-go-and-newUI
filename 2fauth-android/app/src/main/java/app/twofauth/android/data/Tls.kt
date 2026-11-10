package app.twofauth.android.data

import java.net.URL
import java.security.KeyStore
import java.security.MessageDigest
import java.security.cert.CertificateException
import java.security.cert.X509Certificate
import javax.net.ssl.HostnameVerifier
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.SSLPeerUnverifiedException
import javax.net.ssl.TrustManagerFactory
import javax.net.ssl.X509TrustManager
import okhttp3.OkHttpClient

/** SHA-256 of a certificate, as "AB:CD:…", the way browsers show it. */
fun X509Certificate.sha256(): String =
    MessageDigest.getInstance("SHA-256").digest(encoded).joinToString(":") { "%02X".format(it) }

/**
 * A server with a self-signed certificate (a home network instance) can be trusted
 * once its fingerprint has been confirmed: from then on exactly that certificate is
 * accepted for it, and any other one is refused like by a browser.
 */
class PinningTrust(private val pin: String?) : X509TrustManager {
    private val system: X509TrustManager = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm())
        .apply { init(null as KeyStore?) }
        .trustManagers.filterIsInstance<X509TrustManager>().first()

    override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) =
        system.checkClientTrusted(chain, authType)

    override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) {
        if (pin != null && chain.firstOrNull()?.sha256() == pin) return
        try {
            system.checkServerTrusted(chain, authType)
        } catch (error: CertificateException) {
            throw UntrustedCertificate(chain.first(), error)
        }
    }

    override fun getAcceptedIssuers(): Array<X509Certificate> = system.acceptedIssuers

    fun apply(builder: OkHttpClient.Builder): OkHttpClient.Builder {
        val context = SSLContext.getInstance("TLS").apply { init(null, arrayOf(this@PinningTrust), null) }
        val default = HttpsURLConnection.getDefaultHostnameVerifier()
        // A pinned certificate is the proof of identity; its name (often an IP) need not match.
        val verifier = HostnameVerifier { host, session ->
            val leaf = runCatching { session.peerCertificates.firstOrNull() as? X509Certificate }.getOrNull()
            (pin != null && leaf?.sha256() == pin) || default.verify(host, session)
        }
        return builder.sslSocketFactory(context.socketFactory, this).hostnameVerifier(verifier)
    }
}

class UntrustedCertificate(val certificate: X509Certificate, cause: Throwable) :
    CertificateException("Untrusted certificate", cause)

/** Finds the untrusted certificate behind a failed request, wherever it is in the causes. */
fun Throwable.untrustedCertificate(): X509Certificate? {
    var current: Throwable? = this
    while (current != null) {
        if (current is UntrustedCertificate) return current.certificate
        current = current.cause
    }
    return null
}

/** True when the failure was the name of the certificate, which a pin also settles. */
fun Throwable.isHostnameMismatch(): Boolean =
    generateSequence(this) { it.cause }.any { it is SSLPeerUnverifiedException }

/**
 * Reads the certificate a server presents without trusting it, to show its
 * fingerprint. Nothing is sent over this connection.
 */
fun peekCertificate(base: String): X509Certificate? = runCatching {
    var seen: X509Certificate? = null
    val recorder = object : X509TrustManager {
        override fun checkClientTrusted(chain: Array<out X509Certificate>, authType: String) = Unit
        override fun checkServerTrusted(chain: Array<out X509Certificate>, authType: String) {
            seen = chain.firstOrNull()
            throw CertificateException("only looking")
        }
        override fun getAcceptedIssuers(): Array<X509Certificate> = emptyArray()
    }
    val context = SSLContext.getInstance("TLS").apply { init(null, arrayOf(recorder), null) }
    val url = URL(base)
    val socket = context.socketFactory.createSocket(url.host, if (url.port > 0) url.port else 443) as javax.net.ssl.SSLSocket
    socket.soTimeout = 8000
    socket.use { runCatching { it.startHandshake() } }
    seen
}.getOrNull()
