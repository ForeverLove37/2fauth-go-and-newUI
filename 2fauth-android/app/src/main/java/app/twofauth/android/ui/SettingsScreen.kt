package app.twofauth.android.ui

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.biometric.BiometricManager
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.BuildConfig
import app.twofauth.android.R
import app.twofauth.android.UiState

@Composable
fun SettingsScreen(model: AppModel, state: UiState, onSecureChanged: () -> Unit, onBack: () -> Unit) {
    val context = LocalContext.current
    val settings = model.settings
    val user = state.user
    var confirmSignOut by remember { mutableStateOf(false) }
    // Local settings are plain preferences: keep a copy that recomposes.
    var appLock by remember { mutableStateOf(settings.appLock) }
    var lockAfter by remember { mutableIntStateOf(settings.lockAfter) }
    var block by remember { mutableStateOf(settings.blockScreenshots) }
    var clear by remember { mutableIntStateOf(settings.clipboardClear) }
    var split by remember { mutableStateOf(settings.splitDigits) }
    val canLock = remember {
        BiometricManager.from(context).canAuthenticate(LOCK_AUTHENTICATORS) == BiometricManager.BIOMETRIC_SUCCESS
    }

    BackHandler(onBack = onBack)

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding()) {
            Row(Modifier.padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back)) }
                Text(stringResource(R.string.settings), fontWeight = FontWeight.Bold, fontSize = 19.sp)
            }
            Column(
                Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).widthIn(max = 640.dp),
            ) {
                SectionTitle(stringResource(R.string.section_account))
                Column(Modifier.fillMaxWidth().glass().padding(16.dp)) {
                    Text(user?.name.orEmpty(), fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
                    Text(user?.email.orEmpty(), color = Palette.muted, fontSize = 13.sp)
                    Spacer(Modifier.height(10.dp))
                    Text(state.server, fontFamily = Mono, fontSize = 12.sp, color = Palette.muted)
                    Text(
                        stringResource(if (state.viaToken) R.string.connected_token else R.string.connected_session),
                        fontSize = 12.sp,
                        color = Palette.muted,
                    )
                    model.pinOf()?.let {
                        Text(stringResource(R.string.pinned_certificate), fontSize = 12.sp, color = Palette.warn, modifier = Modifier.padding(top = 6.dp))
                        Text(it, fontFamily = Mono, fontSize = 10.sp, color = Palette.muted)
                    }
                }

                SectionTitle(stringResource(R.string.section_security))
                Column(Modifier.fillMaxWidth().glass()) {
                    Toggle(
                        stringResource(R.string.app_lock),
                        stringResource(if (canLock) R.string.app_lock_hint else R.string.app_lock_unavailable),
                        appLock && canLock,
                        enabled = canLock,
                    ) {
                        appLock = it
                        settings.appLock = it
                    }
                    if (appLock && canLock) {
                        Choice(
                            stringResource(R.string.lock_after),
                            lockAfter,
                            listOf(0 to stringResource(R.string.immediately), 30 to "30 s", 60 to "1 min", 300 to "5 min", 900 to "15 min"),
                        ) {
                            lockAfter = it
                            settings.lockAfter = it
                        }
                    }
                    Toggle(stringResource(R.string.block_screenshots), stringResource(R.string.block_screenshots_hint), block) {
                        block = it
                        settings.blockScreenshots = it
                        onSecureChanged()
                    }
                    Choice(
                        stringResource(R.string.clipboard_clear),
                        clear,
                        listOf(0 to stringResource(R.string.never), 15 to "15 s", 30 to "30 s", 60 to "60 s", 120 to "2 min"),
                    ) {
                        clear = it
                        settings.clipboardClear = it
                    }
                }

                SectionTitle(stringResource(R.string.section_display))
                Column(Modifier.fillMaxWidth().glass()) {
                    // These two live on the server and follow the user to the web panel.
                    Toggle(
                        stringResource(R.string.hide_codes),
                        stringResource(R.string.hide_codes_hint),
                        user?.preference("showOtpAsDot", true) ?: true,
                    ) { model.setServerPreference("showOtpAsDot", it) }
                    Toggle(stringResource(R.string.show_next), stringResource(R.string.synced_hint), user?.preference("showNextOtp", true) ?: true) {
                        model.setServerPreference("showNextOtp", it)
                    }
                    Toggle(stringResource(R.string.show_icons), stringResource(R.string.synced_hint), user?.preference("showAccountsIcons", true) ?: true) {
                        model.setServerPreference("showAccountsIcons", it)
                    }
                    Toggle(stringResource(R.string.split_digits), stringResource(R.string.split_digits_hint), split) {
                        split = it
                        settings.splitDigits = it
                    }
                }

                SectionTitle(stringResource(R.string.section_more))
                Column(Modifier.fillMaxWidth().glass()) {
                    SheetAction(Icons.AutoMirrored.Outlined.OpenInNew, stringResource(R.string.open_web)) {
                        context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(state.server)))
                    }
                    SheetAction(Icons.AutoMirrored.Outlined.Logout, stringResource(R.string.sign_out), Palette.danger) { confirmSignOut = true }
                }
                Text(
                    "2FAuth Android ${BuildConfig.VERSION_NAME}" + (state.config?.appVersion?.takeIf { it.isNotEmpty() }?.let { " · server $it" } ?: ""),
                    color = Palette.muted,
                    fontSize = 11.sp,
                    modifier = Modifier.padding(vertical = 24.dp).fillMaxWidth(),
                    textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                )
            }
        }
    }

    if (confirmSignOut) {
        AlertDialog(
            onDismissRequest = { confirmSignOut = false },
            title = { Text(stringResource(R.string.sign_out)) },
            text = { Text(stringResource(R.string.sign_out_text)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmSignOut = false
                    model.signOut()
                }) { Text(stringResource(R.string.sign_out), color = Palette.danger) }
            },
            dismissButton = { TextButton(onClick = { confirmSignOut = false }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.panelStrong,
        )
    }
}

const val LOCK_AUTHENTICATORS = BiometricManager.Authenticators.BIOMETRIC_WEAK or BiometricManager.Authenticators.DEVICE_CREDENTIAL

@Composable
private fun Toggle(title: String, hint: String, checked: Boolean, enabled: Boolean = true, onChange: (Boolean) -> Unit) {
    Row(
        Modifier.fillMaxWidth().clickable(enabled = enabled) { onChange(!checked) }.padding(horizontal = 16.dp, vertical = 12.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, fontSize = 15.sp, color = if (enabled) Palette.text else Palette.muted)
            Text(hint, fontSize = 12.sp, color = Palette.muted)
        }
        Spacer(Modifier.width(12.dp))
        Switch(
            checked = checked,
            onCheckedChange = onChange,
            enabled = enabled,
            colors = SwitchDefaults.colors(
                checkedThumbColor = Palette.accentContrast,
                checkedTrackColor = Palette.accent,
                uncheckedThumbColor = Palette.muted,
                uncheckedTrackColor = Palette.panel,
                uncheckedBorderColor = Palette.border,
            ),
        )
    }
}

@Composable
private fun Choice(title: String, value: Int, options: List<Pair<Int, String>>, onChange: (Int) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().clickable { open = true }.padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(title, fontSize = 15.sp, modifier = Modifier.weight(1f))
        Column {
            Text(options.firstOrNull { it.first == value }?.second ?: "$value", color = Palette.accent, fontSize = 14.sp)
            DropdownMenu(expanded = open, onDismissRequest = { open = false }, containerColor = Palette.panelStrong) {
                options.forEach { (option, label) ->
                    DropdownMenuItem(text = { Text(label) }, onClick = {
                        onChange(option)
                        open = false
                    })
                }
            }
        }
    }
}
