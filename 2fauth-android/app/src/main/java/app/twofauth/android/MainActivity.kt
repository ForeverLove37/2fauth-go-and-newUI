package app.twofauth.android

import android.Manifest
import android.content.Intent
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.view.WindowManager
import androidx.activity.SystemBarStyle
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.biometric.BiometricPrompt
import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.ContentTransform
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Cloud
import androidx.compose.material.icons.outlined.Fingerprint
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
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
import app.twofauth.android.ui.AddScreen
import app.twofauth.android.ui.Authenticator
import app.twofauth.android.ui.Backdrop
import app.twofauth.android.ui.BrandMark
import app.twofauth.android.ui.CloudScreen
import app.twofauth.android.ui.HomeScreen
import app.twofauth.android.ui.LOCK_AUTHENTICATORS
import app.twofauth.android.ui.LocalAuthenticator
import app.twofauth.android.ui.LocalIcons
import app.twofauth.android.ui.LoginScreen
import app.twofauth.android.ui.Palette
import app.twofauth.android.ui.SettingsScreen
import app.twofauth.android.ui.TwoFAuthTheme
import app.twofauth.android.ui.primaryButton
import coil.ImageLoader
import coil.decode.SvgDecoder
import kotlin.coroutines.resume
import kotlinx.coroutines.launch
import kotlinx.coroutines.suspendCancellableCoroutine

/** The screens; the order is the depth, which sets the direction of the transitions. */
private enum class Screen { Home, Cloud, Settings, Add, Connect }

class MainActivity : FragmentActivity() {
    private val model: AppModel by viewModels()
    /** An otpauth:// link another app opened us with, kept until it can be added. */
    private var incoming by mutableStateOf<String?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        applyBars()
        super.onCreate(savedInstanceState)
        applySecureFlag()
        handle(intent)

        ProcessLifecycleOwner.get().lifecycle.addObserver(object : DefaultLifecycleObserver {
            override fun onStart(owner: LifecycleOwner) = model.onForeground()
            override fun onStop(owner: LifecycleOwner) = model.onBackground()
        })

        val authenticator = Authenticator { title -> verify(title) }

        setContent {
            TwoFAuthTheme {
                val state by model.state.collectAsStateWithLifecycle()
                val accounts by model.store.accounts.collectAsState()
                val loaded by model.store.loaded.collectAsState()
                var screen by rememberSaveable { mutableStateOf(Screen.Home) }
                var returnTo by rememberSaveable { mutableStateOf(Screen.Home) }
                val context = LocalContext.current
                val http = model.http()
                // SVG logos, through the client of the server and its certificate trust.
                val icons = remember(http) {
                    ImageLoader.Builder(context)
                        .components { add(SvgDecoder.Factory()) }
                        .apply { if (http != null) okHttpClient(http) }
                        .crossfade(Palette.motion)
                        .build()
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
                LaunchedEffect(Palette.light) { applyBars() }
                val link = incoming
                LaunchedEffect(link, loaded) { if (link != null && loaded) screen = Screen.Add }
                // Back where the user came from once signed in.
                LaunchedEffect(state.user) { if (state.user != null && screen == Screen.Connect) screen = returnTo }
                AskNotificationsOnce()

                CompositionLocalProvider(LocalIcons provides icons, LocalAuthenticator provides authenticator) {
                    Box(Modifier.fillMaxSize()) {
                        when {
                            !state.ready || !loaded -> Backdrop {}
                            state.locked -> LockScreen { unlock() }
                            else -> Column(Modifier.fillMaxSize()) {
                                Box(Modifier.weight(1f)) {
                                    AnimatedContent(screen, transitionSpec = { transition(initialState, targetState) }, label = "screen") { current ->
                                        when (current) {
                                            Screen.Home -> HomeScreen(model, state, accounts, onAdd = { screen = Screen.Add }, onSettings = { screen = Screen.Settings })
                                            Screen.Cloud -> CloudScreen(
                                                model, state,
                                                onConnect = { returnTo = Screen.Cloud; screen = Screen.Connect },
                                                onSettings = { screen = Screen.Settings },
                                            )
                                            Screen.Settings -> SettingsScreen(
                                                model, state,
                                                onConnect = { returnTo = Screen.Settings; screen = Screen.Connect },
                                                onSecureChanged = { applySecureFlag(); applyBars() },
                                            ) { screen = Screen.Home }
                                            Screen.Add -> AddScreen(model, state, link) {
                                                incoming = null
                                                screen = Screen.Home
                                            }
                                            Screen.Connect -> LoginScreen(model, state) { screen = returnTo }
                                        }
                                    }
                                }
                                AnimatedVisibility(screen == Screen.Home || screen == Screen.Cloud, enter = slideInVertically { it }, exit = slideOutVertically { it }) {
                                    Tabs(screen) { screen = it }
                                }
                            }
                        }
                        SnackbarHost(snackbar, Modifier.align(Alignment.BottomCenter).navigationBarsPadding().padding(bottom = 96.dp))
                    }
                    state.update?.let { UpdateDialog(model, it, state.downloading) }
                }
                LaunchedEffect(state.locked) { if (state.locked) unlock() }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handle(intent)
    }

    private fun handle(intent: Intent?) {
        intent?.data?.toString()?.takeIf { intent.action == Intent.ACTION_VIEW && it.startsWith("otpauth", true) }?.let { incoming = it }
        if (intent?.getBooleanExtra(EXTRA_UPDATE, false) == true) model.checkUpdate(manual = true)
    }

    /** Light icons on dark themes, dark icons on light ones. */
    private fun applyBars() {
        val style = if (Palette.light) SystemBarStyle.light(Color.TRANSPARENT, Color.TRANSPARENT) else SystemBarStyle.dark(Color.TRANSPARENT)
        enableEdgeToEdge(style, style)
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
                .setAllowedAuthenticators(LOCK_AUTHENTICATORS)
                .build(),
        )
    }

    /** One verification, for an action that sends a secret to the server. */
    private suspend fun verify(title: String): Boolean = suspendCancellableCoroutine { continuation ->
        val prompt = BiometricPrompt(this, ContextCompat.getMainExecutor(this), object : BiometricPrompt.AuthenticationCallback() {
            override fun onAuthenticationSucceeded(result: BiometricPrompt.AuthenticationResult) {
                if (continuation.isActive) continuation.resume(true)
            }

            override fun onAuthenticationError(errorCode: Int, errString: CharSequence) {
                if (continuation.isActive) continuation.resume(false)
            }
        })
        prompt.authenticate(
            BiometricPrompt.PromptInfo.Builder()
                .setTitle(title)
                .setSubtitle(getString(R.string.verify_subtitle))
                .setAllowedAuthenticators(LOCK_AUTHENTICATORS)
                .build(),
        )
    }

    companion object {
        const val EXTRA_UPDATE = "app.twofauth.android.UPDATE"
    }
}

private fun transition(from: Screen, to: Screen): ContentTransform {
    if (!Palette.motion) return fadeIn(tween(0)) togetherWith fadeOut(tween(0))
    // The two tabs cross-fade; deeper screens slide in from the right and back out.
    if (from.ordinal <= 1 && to.ordinal <= 1) return fadeIn(tween(220)) togetherWith fadeOut(tween(160))
    val forward = to.ordinal > from.ordinal
    return (slideInHorizontally(tween(280)) { if (forward) it / 3 else -it / 3 } + fadeIn(tween(240)))
        .togetherWith(slideOutHorizontally(tween(240)) { if (forward) -it / 4 else it / 3 } + fadeOut(tween(180)))
}

@Composable
private fun Tabs(screen: Screen, onSelect: (Screen) -> Unit) {
    NavigationBar(containerColor = Palette.sheet, tonalElevation = 0.dp) {
        val colors = NavigationBarItemDefaults.colors(
            selectedIconColor = Palette.accent,
            selectedTextColor = Palette.accent,
            indicatorColor = Palette.accent.copy(alpha = 0.16f),
            unselectedIconColor = Palette.muted,
            unselectedTextColor = Palette.muted,
        )
        NavigationBarItem(
            selected = screen == Screen.Home, onClick = { onSelect(Screen.Home) },
            icon = { Icon(Icons.Outlined.PhoneAndroid, null) }, label = { Text(stringResource(R.string.tab_codes)) }, colors = colors,
        )
        NavigationBarItem(
            selected = screen == Screen.Cloud, onClick = { onSelect(Screen.Cloud) },
            icon = { Icon(Icons.Outlined.Cloud, null) }, label = { Text(stringResource(R.string.tab_cloud)) }, colors = colors,
        )
    }
}

/** Updates are announced by a notification: ask for the permission once, on Android 13 and later. */
@Composable
private fun AskNotificationsOnce() {
    if (Build.VERSION.SDK_INT < 33) return
    val context = LocalContext.current
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}
    LaunchedEffect(Unit) {
        val prefs = context.getSharedPreferences("updates", android.content.Context.MODE_PRIVATE)
        if (!prefs.getBoolean("asked", false)) {
            prefs.edit().putBoolean("asked", true).apply()
            launcher.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
    }
}

@Composable
private fun UpdateDialog(model: AppModel, release: app.twofauth.android.data.AndroidRelease, progress: Float?) {
    AlertDialog(
        onDismissRequest = { if (progress == null) model.dismissUpdate() },
        icon = { BrandMark(40.dp) },
        title = { Text(stringResource(R.string.update_available, release.versionName)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                Text(stringResource(R.string.update_size, release.size / 1024f / 1024f), color = Palette.muted, fontSize = 12.sp)
                if (release.notes.isNotBlank()) Text(release.notes, fontSize = 14.sp, modifier = Modifier.padding(top = 10.dp))
                if (progress != null) {
                    Spacer(Modifier.height(14.dp))
                    LinearProgressIndicator(progress = { progress }, color = Palette.accent, trackColor = Palette.border, modifier = Modifier.fillMaxWidth())
                }
            }
        },
        confirmButton = {
            TextButton(enabled = progress == null, onClick = model::installUpdate) {
                Text(stringResource(if (progress == null) R.string.update_install else R.string.update_downloading), color = Palette.accent)
            }
        },
        dismissButton = { if (progress == null) TextButton(onClick = model::dismissUpdate) { Text(stringResource(R.string.update_later)) } },
        containerColor = Palette.sheet,
    )
}

@Composable
private fun LockScreen(onUnlock: () -> Unit) {
    Backdrop {
        Column(
            Modifier.fillMaxSize().padding(32.dp),
            verticalArrangement = Arrangement.Center,
            horizontalAlignment = Alignment.CenterHorizontally,
        ) {
            BrandMark(88.dp)
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
