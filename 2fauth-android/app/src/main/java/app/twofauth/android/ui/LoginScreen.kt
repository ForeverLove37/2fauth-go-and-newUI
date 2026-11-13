package app.twofauth.android.ui

import android.annotation.SuppressLint
import android.graphics.Bitmap
import android.net.Uri
import android.net.http.SslError
import android.os.Build
import android.webkit.CookieManager
import android.webkit.SslErrorHandler
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material.icons.outlined.WarningAmber
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.viewinterop.AndroidView
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.sha256
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate

private enum class Method { Password, Token }

@Composable
fun LoginScreen(model: AppModel, state: UiState, onBack: () -> Unit) {
    var sso by remember { mutableStateOf(false) }
    if (sso && state.config != null) {
        SsoScreen(state.server, model.pinOf(), onCookies = {
            sso = false
            model.signInWithCookies(it)
        }, onFail = {
            sso = false
            model.say(it)
        })
        return
    }

    BackHandler(onBack = onBack)
    Backdrop {
        Column(
            Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().imePadding().verticalScroll(rememberScrollState()).padding(24.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            Row(Modifier.fillMaxWidth()) {
                IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back)) }
            }
            Spacer(Modifier.height(12.dp))
            BrandMark(64.dp)
            Spacer(Modifier.height(18.dp))
            Text(state.config?.appName ?: "2FAuth", fontSize = 26.sp, fontWeight = FontWeight.Bold)
            Text(stringResource(R.string.tagline).uppercase(), fontSize = 11.sp, letterSpacing = 1.6.sp, color = Palette.muted)
            Spacer(Modifier.height(36.dp))

            Column(Modifier.widthIn(max = 460.dp).fillMaxWidth().glass().padding(20.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
                // First in the card: below the button it would be under the keyboard.
                state.error?.let {
                    Text(
                        it,
                        color = Palette.danger,
                        fontSize = 13.sp,
                        modifier = Modifier.fillMaxWidth().glass(SmallShape, Palette.danger.copy(alpha = 0.45f)).padding(10.dp),
                    )
                }
                if (state.config == null) ServerStep(model, state) else SignInStep(model, state) { sso = true }
            }
            Spacer(Modifier.height(18.dp))
            Text(stringResource(R.string.login_footer), color = Palette.muted, fontSize = 12.sp, textAlign = TextAlign.Center)
        }
    }

    state.certificate?.let { prompt ->
        AlertDialog(
            onDismissRequest = model::dismissCertificate,
            icon = { Icon(Icons.Outlined.WarningAmber, null, tint = Palette.warn) },
            title = { Text(stringResource(R.string.cert_title)) },
            text = {
                Column {
                    Text(stringResource(if (prompt.hostnameOnly) R.string.cert_text_name else R.string.cert_text))
                    Text(prompt.subject, fontSize = 12.sp, color = Palette.muted, modifier = Modifier.padding(top = 10.dp))
                    Text("SHA-256", fontSize = 11.sp, color = Palette.muted, modifier = Modifier.padding(top = 10.dp))
                    Text(prompt.fingerprint, fontFamily = Mono, fontSize = 12.sp)
                }
            },
            confirmButton = { TextButton(onClick = { model.trustCertificate(prompt) }) { Text(stringResource(R.string.cert_trust), color = Palette.warn) } },
            dismissButton = { TextButton(onClick = model::dismissCertificate) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.sheet,
        )
    }
}

@Composable
private fun ServerStep(model: AppModel, state: UiState) {
    var server by rememberSaveable { mutableStateOf(state.server.removePrefix("https://")) }
    Text(stringResource(R.string.server_title), fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
    OutlinedTextField(
        value = server,
        onValueChange = { server = it.trim(); model.clearError() },
        label = { Text(stringResource(R.string.server_label)) },
        placeholder = { Text("2fa.example.com") },
        singleLine = true,
        colors = fieldColors(),
        shape = RoundedCornerShape(14.dp),
        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri, imeAction = ImeAction.Go, autoCorrectEnabled = false),
        keyboardActions = KeyboardActions(onGo = { if (server.isNotBlank()) model.connect(server) }),
        modifier = Modifier.fillMaxWidth(),
    )
    if (server.startsWith("http://", true)) {
        Text(stringResource(R.string.http_warning), color = Palette.warn, fontSize = 12.sp)
    }
    BusyButton(stringResource(R.string.continue_action), state.busy, server.isNotBlank()) { model.connect(server) }
}

@Composable
private fun SignInStep(model: AppModel, state: UiState, onSso: () -> Unit) {
    val config = state.config ?: return
    val ssoOnly = config.appSettings.useSsoOnly
    var method by rememberSaveable { mutableStateOf(Method.Password) }
    var email by rememberSaveable { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var token by remember { mutableStateOf("") }
    var showPassword by remember { mutableStateOf(false) }
    // On a single sign-on only instance the password is the administrators' way back in.
    var passwordOpen by rememberSaveable { mutableStateOf(!ssoOnly) }

    Row(verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = model::resetServer, modifier = Modifier.size(32.dp)) {
            Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.change_server), tint = Palette.muted)
        }
        Text(Uri.parse(state.server).host ?: state.server, color = Palette.muted, fontSize = 13.sp, fontFamily = Mono)
    }

    if (config.sso.openid) {
        Button(onClick = onSso, enabled = !state.busy, colors = primaryButton(), modifier = Modifier.fillMaxWidth().height(50.dp)) {
            Text(stringResource(R.string.sign_in_sso))
        }
    }

    if (method == Method.Token) {
        OutlinedTextField(
            value = token,
            onValueChange = { token = it.trim() },
            label = { Text(stringResource(R.string.token_label)) },
            minLines = 3,
            colors = fieldColors(),
            shape = RoundedCornerShape(14.dp),
            textStyle = androidx.compose.ui.text.TextStyle(fontFamily = Mono, fontSize = 12.sp),
            modifier = Modifier.fillMaxWidth(),
        )
        Text(stringResource(R.string.token_help), color = Palette.muted, fontSize = 12.sp)
        BusyButton(stringResource(R.string.sign_in), state.busy, token.isNotBlank()) { model.signInWithToken(token) }
        TextButton(onClick = { method = Method.Password }) { Text(stringResource(R.string.use_password)) }
        return
    }

    if (!passwordOpen) {
        TextButton(onClick = { passwordOpen = true }) { Text(stringResource(R.string.admin_password)) }
    } else {
        OutlinedTextField(
            value = email,
            onValueChange = { email = it.trim() },
            label = { Text(stringResource(R.string.email)) },
            singleLine = true,
            colors = fieldColors(),
            shape = RoundedCornerShape(14.dp),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Email, imeAction = ImeAction.Next, autoCorrectEnabled = false),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text(stringResource(R.string.password)) },
            singleLine = true,
            visualTransformation = if (showPassword) VisualTransformation.None else PasswordVisualTransformation(),
            trailingIcon = {
                IconButton(onClick = { showPassword = !showPassword }) {
                    Icon(if (showPassword) Icons.Outlined.VisibilityOff else Icons.Outlined.Visibility, null, tint = Palette.muted)
                }
            },
            colors = fieldColors(),
            shape = RoundedCornerShape(14.dp),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Done),
            keyboardActions = KeyboardActions(onDone = { if (email.isNotBlank() && password.isNotEmpty()) model.signInWithPassword(email, password) }),
            modifier = Modifier.fillMaxWidth(),
        )
        if (config.sso.openid) {
            OutlinedButton(
                onClick = { model.signInWithPassword(email, password) },
                enabled = !state.busy && email.isNotBlank() && password.isNotEmpty(),
                modifier = Modifier.fillMaxWidth().height(50.dp),
            ) { Text(stringResource(R.string.sign_in)) }
        } else {
            BusyButton(stringResource(R.string.sign_in), state.busy, email.isNotBlank() && password.isNotEmpty()) {
                model.signInWithPassword(email, password)
            }
        }
    }
    if (!ssoOnly || config.appSettings.allowPatWhileSsoOnly) {
        TextButton(onClick = { method = Method.Token }) { Text(stringResource(R.string.use_token)) }
    }
}

@Composable
private fun BusyButton(text: String, busy: Boolean, enabled: Boolean, onClick: () -> Unit) {
    Button(onClick = onClick, enabled = enabled && !busy, colors = primaryButton(), modifier = Modifier.fillMaxWidth().height(50.dp)) {
        if (busy) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp, color = Palette.accentContrast) else Text(text)
    }
}

/**
 * Single sign-on runs in a web view, the only place where its cookies can be picked
 * up: once the provider sends the browser back to the panel, the session is in them.
 */
@SuppressLint("SetJavaScriptEnabled")
@Composable
private fun SsoScreen(server: String, pin: String?, onCookies: (Map<String, String>) -> Unit, onFail: (String) -> Unit) {
    val base = Uri.parse(server)
    val basePath = base.path.orEmpty().trimEnd('/')
    var loading by remember { mutableStateOf(true) }
    var done by remember { mutableStateOf(false) }
    val failed = stringResource(R.string.sso_failed)
    val cancelled = stringResource(R.string.sso_cancelled)

    BackHandler { onFail(cancelled) }

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding()) {
            Row(Modifier.padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { onFail(cancelled) }) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back)) }
                Text(stringResource(R.string.sign_in_sso), fontWeight = FontWeight.SemiBold)
                Spacer(Modifier.weight(1f))
                if (loading) CircularProgressIndicator(Modifier.size(20.dp).padding(end = 4.dp), strokeWidth = 2.dp, color = Palette.accent)
                Spacer(Modifier.size(16.dp))
            }
            Box(Modifier.fillMaxSize()) {
                AndroidView(
                    modifier = Modifier.fillMaxSize(),
                    factory = { context ->
                        val cookies = CookieManager.getInstance()
                        cookies.removeAllCookies(null)
                        WebView(context).apply {
                            settings.javaScriptEnabled = true
                            settings.domStorageEnabled = true
                            settings.userAgentString = settings.userAgentString + " 2FAuth-Android"
                            webViewClient = object : WebViewClient() {
                                fun check(url: String?): Boolean {
                                    val target = url?.let(Uri::parse) ?: return false
                                    if (done || target.host != base.host) return false
                                    val path = target.path.orEmpty().removePrefix(basePath)
                                    if (path.startsWith("/socialite")) return false
                                    if (path.startsWith("/login")) {
                                        if (target.getQueryParameter("sso_error") != null) {
                                            done = true
                                            onFail(failed + " (" + target.getQueryParameter("sso_error") + ")")
                                            return true
                                        }
                                        return false
                                    }
                                    // Back on the panel: signed in.
                                    done = true
                                    val jar = cookies.getCookie(server).orEmpty()
                                    cookies.removeAllCookies(null)
                                    onCookies(
                                        jar.split(";").mapNotNull { part ->
                                            val (name, value) = part.trim().split("=", limit = 2).takeIf { it.size == 2 } ?: return@mapNotNull null
                                            name to value
                                        }.toMap(),
                                    )
                                    return true
                                }

                                override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean =
                                    check(request.url.toString())

                                override fun onPageStarted(view: WebView, url: String?, favicon: Bitmap?) {
                                    loading = true
                                    if (check(url)) view.stopLoading()
                                }

                                override fun onPageFinished(view: WebView, url: String?) {
                                    loading = false
                                }

                                // The certificate the user confirmed for this server, and nothing else.
                                override fun onReceivedSslError(view: WebView, handler: SslErrorHandler, error: SslError) {
                                    val certificate = error.certificate.x509()
                                    if (pin != null && certificate?.sha256() == pin && Uri.parse(error.url).host == base.host) handler.proceed()
                                    else handler.cancel()
                                }
                            }
                            loadUrl("$server/socialite/redirect/openid")
                        }
                    },
                    onRelease = { it.destroy() },
                )
            }
        }
    }
}

private fun android.net.http.SslCertificate.x509(): X509Certificate? =
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
        x509Certificate
    } else {
        runCatching {
            val bytes = android.net.http.SslCertificate.saveState(this).getByteArray("x509-certificate") ?: return null
            CertificateFactory.getInstance("X.509").generateCertificate(bytes.inputStream()) as X509Certificate
        }.getOrNull()
    }
