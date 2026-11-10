package app.twofauth.android

import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.compose.setContent
import androidx.activity.SystemBarStyle
import androidx.activity.enableEdgeToEdge
import androidx.activity.viewModels
import androidx.biometric.BiometricPrompt
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.ContextCompat
import androidx.fragment.app.FragmentActivity
import androidx.lifecycle.DefaultLifecycleObserver
import androidx.lifecycle.LifecycleOwner
import androidx.lifecycle.ProcessLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import app.twofauth.android.ui.AccountsScreen
import app.twofauth.android.ui.AddScreen
import app.twofauth.android.ui.Backdrop
import app.twofauth.android.ui.BrandMark
import app.twofauth.android.ui.LOCK_AUTHENTICATORS
import app.twofauth.android.ui.LocalIcons
import app.twofauth.android.ui.LoginScreen
import app.twofauth.android.ui.Palette
import app.twofauth.android.ui.SettingsScreen
import app.twofauth.android.ui.TwoFAuthTheme
import app.twofauth.android.ui.primaryButton
import coil.ImageLoader
import kotlinx.coroutines.launch

private enum class Screen { Accounts, Settings, Add }

class MainActivity : FragmentActivity() {
    private val model: AppModel by viewModels()
    /** An otpauth:// link another app opened us with, kept until it can be added. */
    private var incoming by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        // The panel is always dark: light status bar icons whatever the system theme.
        enableEdgeToEdge(SystemBarStyle.dark(Color.TRANSPARENT), SystemBarStyle.dark(Color.TRANSPARENT))
        super.onCreate(savedInstanceState)
        applySecureFlag()
        incoming = otpauthOf(intent)

        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) = model.onForeground()
            override fun onStop(owner: LifecycleOwner) = model.onBackground()
        })

        setContent {
            TwoFAuthTheme {
                val state by model.state.collectAsStateWithLifecycle()
                var screen by rememberSaveable { mutableStateOf(Screen.Accounts) }
                val http = model.http()
                val context = LocalContext.current
                // Icons go through the client of the server, and so through its certificate trust.
                val icons = remember(http) {
                    ImageLoader.Builder(context).apply { if (http != null) okHttpClient(http) }.crossfade(true).build()
                }
                val link = incoming
                // Signing in again starts from the codes, not from where the last session ended.
                LaunchedEffect(state.user == null) {
                    if (state.user == null) screen = Screen.Accounts
                }
                LaunchedEffect(link, state.user) {
                    if (link != null && state.user != null) screen = Screen.Add
                }

                val snackbar = remember { SnackbarHostState() }
                val scope = rememberCoroutineScope()
                LaunchedEffect(state.message) {
                    val message = state.message ?: return@LaunchedEffect
                    // Not in this effect: consuming the message restarts it, which would cancel the display.
                    scope.launch {
                        snackbar.currentSnackbarData?.dismiss()
                        snackbar.showSnackbar(message)
                    }
                    model.consumeMessage()
                }

                CompositionLocalProvider(LocalIcons provides icons) {
                    Box(Modifier.fillMaxSize()) {
                        when {
                            !state.ready -> Backdrop {}
                            state.user == null -> LoginScreen(model, state)
                            state.locked -> LockScreen { unlock() }
                            else -> AnimatedContent(screen, transitionSpec = { fadeIn() togetherWith fadeOut() }, label = "screen") { current ->
                                when (current) {
                                    Screen.Accounts -> AccountsScreen(model, state, onSettings = { screen = Screen.Settings }, onAdd = { screen = Screen.Add })
                                    Screen.Settings -> SettingsScreen(model, state, onSecureChanged = ::applySecureFlag) { screen = Screen.Accounts }
                                    Screen.Add -> AddScreen(model, state.groups, link) {
                                        incoming = null
                                        screen = Screen.Accounts
                                    }
                                }
                            }
                        }
                        // Above every screen, clear of the add button.
                        SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 84.dp))
                    }
                }
                LaunchedEffect(state.locked) { if (state.locked) unlock() }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        otpauthOf(intent)?.let { incoming = it }
    }

    /** Screenshots, screen recording and the recent apps preview see a blank window. */
    private fun applySecureFlag() {
        if (model.settings.blockScreenshots) window.addFlags(WindowManager.LayoutParams.FLAG_SECURE)
        else window.clearFlags(WindowManager.LayoutParams.FLAG_SECURE)
    }

    private fun unlock() {
        val prompt = BiometricPrompt(this, ContextCompat.getMainExecutor(this), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) = model.unlock()
        })
        prompt.authenticate(
            BiometricPrompt.PromptInfo.Builder()
                .setTitle(getString(R.string.unlock_title))
                .setSubtitle(model.state.value.config?.appName ?: getString(R.string.app_name))
                .setAllowedAuthenticators(LOCK_AUTHENTICATORS)
                .build(),
        )
    }

    private fun otpauthOf(intent: Intent?): String? =
        intent?.data?.toString()?.takeIf { intent.action == Intent.ACTION_VIEW && it.startsWith("otpauth", true) }
}

@Composable
private fun LockScreen(onUnlock: () -> Unit) {
    Backdrop {
        Column(
            Modifier.fillMaxSize().padding(32.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            BrandMark(72.dp)
            Spacer(Modifier.height(24.dp))
            Text(stringResource(R.string.locked), fontSize = 22.sp, fontWeight = FontWeight.Bold)
            Text(stringResource(R.string.locked_hint), color = Palette.muted, fontSize = 14.sp)
            Spacer(Modifier.height(32.dp))
            Button(onClick = onUnlock, colors = primaryButton()) {
                Icon(Icons.Outlined.Fingerprint, null)
                Spacer(Modifier.padding(4.dp))
                Text(stringResource(R.string.unlock))
            }
        }
    }
}
