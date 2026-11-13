package app.twofauth.android.ui

import android.Manifest
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.text.format.DateUtils
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.animateColorAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.automirrored.outlined.Logout
import androidx.compose.material.icons.automirrored.outlined.OpenInNew
import androidx.compose.material.icons.outlined.Check
import androidx.compose.material.icons.outlined.CloudSync
import androidx.compose.material.icons.outlined.Link
import androidx.compose.material.icons.outlined.SystemUpdate
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
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.BuildConfig
import app.twofauth.android.R
import app.twofauth.android.TwoFAuthApp
import app.twofauth.android.UiState
import app.twofauth.android.data.Kind
import app.twofauth.android.data.Updates
import kotlinx.coroutines.launch

@Composable
fun SettingsScreen(model: AppModel, state: UiState, onConnect: () -> Unit, onSecureChanged: () -> Unit, onBack: () -> Unit) {
    val context = LocalContext.current
    val app = context.applicationContext as TwoFAuthApp
    val settings = model.settings
    val scope = rememberCoroutineScope()
    val authenticator = LocalAuthenticator.current
    var themeId by remember { mutableStateOf(settings.themeId) }
    var accent by remember { mutableLongStateOf(settings.accent) }
    var cardStyle by remember { mutableStateOf(settings.cardStyle) }
    var animations by remember { mutableStateOf(settings.animations) }
    var defaultKind by remember { mutableStateOf(settings.defaultKind) }
    var hide by remember { mutableStateOf(settings.hideCodes) }
    var showNext by remember { mutableStateOf(settings.showNext) }
    var showIcons by remember { mutableStateOf(settings.showIcons) }
    var publicIcons by remember { mutableStateOf(settings.publicIcons) }
    var split by remember { mutableStateOf(settings.splitDigits) }
    var appLock by remember { mutableStateOf(settings.appLock) }
    var lockAfter by remember { mutableIntStateOf(settings.lockAfter) }
    var block by remember { mutableStateOf(settings.blockScreenshots) }
    var clear by remember { mutableIntStateOf(settings.clipboardClear) }
    var checkUpdates by remember { mutableStateOf(settings.checkUpdates) }
    var confirmSignOut by remember { mutableStateOf(false) }
    val canLock = remember { model.canVerifyUser() }
    val verifyTitle = stringResource(R.string.verify_default)
    val notifications = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) {}

    fun retheme() {
        settings.themeId = themeId
        settings.accent = accent
        settings.cardStyle = cardStyle
        settings.animations = animations
        app.applyTheme()
        onSecureChanged()
    }

    BackHandler(onBack = onBack)

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding()) {
            Row(Modifier.padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = onBack) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back)) }
                Text(stringResource(R.string.settings), fontWeight = FontWeight.Bold, fontSize = 19.sp)
            }
            Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).widthIn(max = 680.dp)) {
                // ------------------------------------------------------------ server
                SectionTitle(stringResource(R.string.section_server))
                Column(Modifier.fillMaxWidth().glass()) {
                    if (state.user == null) {
                        Text(stringResource(R.string.server_offline), color = Palette.muted, fontSize = 13.sp, modifier = Modifier.padding(16.dp))
                        SheetAction(Icons.Outlined.Link, stringResource(R.string.connect_server), Palette.accent, onConnect)
                    } else {
                        Column(Modifier.padding(16.dp)) {
                            Text(state.user.name, fontWeight = FontWeight.SemiBold, fontSize = 16.sp)
                            Text(state.user.email, color = Palette.muted, fontSize = 13.sp)
                            Text(state.server, fontFamily = Mono, fontSize = 12.sp, color = Palette.muted, modifier = Modifier.padding(top = 8.dp))
                            Text(
                                when {
                                    state.syncing -> stringResource(R.string.syncing)
                                    state.syncError != null -> stringResource(R.string.sync_failed, state.syncError)
                                    state.lastSync > 0 && System.currentTimeMillis() - state.lastSync < 60_000 -> stringResource(R.string.sync_just_now)
                                    state.lastSync > 0 -> stringResource(R.string.last_sync, DateUtils.getRelativeTimeSpanString(state.lastSync).toString())
                                    else -> stringResource(R.string.never_synced)
                                },
                                fontSize = 12.sp,
                                color = if (state.syncError != null) Palette.warn else Palette.muted,
                            )
                            model.pinOf()?.let {
                                Text(stringResource(R.string.pinned_certificate), fontSize = 12.sp, color = Palette.warn, modifier = Modifier.padding(top = 6.dp))
                                Text(it, fontFamily = Mono, fontSize = 10.sp, color = Palette.muted)
                            }
                        }
                        SheetAction(Icons.Outlined.CloudSync, stringResource(R.string.sync_now)) { model.sync(manual = true) }
                        SheetAction(Icons.AutoMirrored.Outlined.OpenInNew, stringResource(R.string.open_web)) {
                            context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(state.server)))
                        }
                        SheetAction(Icons.AutoMirrored.Outlined.Logout, stringResource(R.string.sign_out), Palette.danger) { confirmSignOut = true }
                    }
                }

                // ------------------------------------------------------------ appearance
                SectionTitle(stringResource(R.string.section_appearance))
                Column(Modifier.fillMaxWidth().glass().padding(vertical = 14.dp)) {
                    Text(stringResource(R.string.theme), fontSize = 15.sp, modifier = Modifier.padding(horizontal = 16.dp))
                    LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 10.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        items(themePresets, key = { it.id }) { preset ->
                            ThemeSwatch(preset, selected = preset.id == themeId) {
                                themeId = preset.id
                                retheme()
                            }
                        }
                    }
                    Text(stringResource(R.string.accent), fontSize = 15.sp, modifier = Modifier.padding(horizontal = 16.dp, vertical = 4.dp))
                    LazyRow(contentPadding = PaddingValues(horizontal = 16.dp, vertical = 8.dp), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                        items(accentChoices) { value ->
                            val preset = themePresets.firstOrNull { it.id == themeId } ?: themePresets.first()
                            AccentDot(Color(if (value == 0L) preset.accent else value), selected = accent == value, auto = value == 0L) {
                                accent = value
                                retheme()
                            }
                        }
                    }
                    Text(stringResource(R.string.card_style), fontSize = 15.sp, modifier = Modifier.padding(horizontal = 16.dp, vertical = 6.dp))
                    LazyRow(contentPadding = PaddingValues(horizontal = 16.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        items(CardStyle.entries) { style ->
                            StyleChip(styleLabel(style), selected = cardStyle == style.name) {
                                cardStyle = style.name
                                retheme()
                            }
                        }
                    }
                    Box(Modifier.padding(16.dp)) { PreviewCard() }
                    Toggle(stringResource(R.string.animations), stringResource(R.string.animations_hint), animations) {
                        animations = it
                        retheme()
                    }
                }

                // ------------------------------------------------------------ codes
                SectionTitle(stringResource(R.string.section_codes))
                Column(Modifier.fillMaxWidth().glass()) {
                    Choice(
                        stringResource(R.string.default_kind),
                        defaultKind.name,
                        listOf(Kind.LOCAL.name to stringResource(R.string.kind_local), Kind.SYNCED.name to stringResource(R.string.kind_synced)),
                    ) { picked ->
                        val kind = Kind.valueOf(picked)
                        if (kind == Kind.SYNCED) {
                            // Syncing by default sends every new secret to the server: verify first.
                            scope.launch {
                                if (!model.canVerifyUser()) {
                                    model.say(context.getString(R.string.need_screen_lock))
                                } else if (authenticator.verify(verifyTitle)) {
                                    defaultKind = kind
                                    settings.defaultKind = kind
                                }
                            }
                        } else {
                            defaultKind = kind
                            settings.defaultKind = kind
                        }
                    }
                    Text(stringResource(R.string.default_kind_hint), color = Palette.muted, fontSize = 12.sp, modifier = Modifier.padding(start = 16.dp, end = 16.dp, bottom = 8.dp))
                    Toggle(stringResource(R.string.hide_codes), stringResource(R.string.hide_codes_hint), hide) { hide = it; settings.hideCodes = it }
                    Toggle(stringResource(R.string.show_next), stringResource(R.string.show_next_hint), showNext) { showNext = it; settings.showNext = it }
                    Toggle(stringResource(R.string.split_digits), stringResource(R.string.split_digits_hint), split) { split = it; settings.splitDigits = it }
                    Toggle(stringResource(R.string.show_icons), stringResource(R.string.show_icons_hint), showIcons) { showIcons = it; settings.showIcons = it }
                    Toggle(stringResource(R.string.public_icons), stringResource(R.string.public_icons_hint), publicIcons) {
                        publicIcons = it
                        settings.publicIcons = it
                        model.iconMemo.clear()
                    }
                }

                // ------------------------------------------------------------ security
                SectionTitle(stringResource(R.string.section_security))
                Column(Modifier.fillMaxWidth().glass()) {
                    Toggle(
                        stringResource(R.string.app_lock),
                        stringResource(if (canLock) R.string.app_lock_hint else R.string.app_lock_unavailable),
                        appLock && canLock,
                        enabled = canLock,
                    ) { appLock = it; settings.appLock = it }
                    if (appLock && canLock) {
                        Choice(
                            stringResource(R.string.lock_after),
                            lockAfter.toString(),
                            listOf("0" to stringResource(R.string.immediately), "30" to "30 s", "60" to "1 min", "300" to "5 min", "900" to "15 min"),
                        ) { lockAfter = it.toInt(); settings.lockAfter = it.toInt() }
                    }
                    Toggle(stringResource(R.string.block_screenshots), stringResource(R.string.block_screenshots_hint), block) {
                        block = it
                        settings.blockScreenshots = it
                        onSecureChanged()
                    }
                    Choice(
                        stringResource(R.string.clipboard_clear),
                        clear.toString(),
                        listOf("0" to stringResource(R.string.never), "15" to "15 s", "30" to "30 s", "60" to "60 s", "120" to "2 min"),
                    ) { clear = it.toInt(); settings.clipboardClear = it.toInt() }
                }

                // ------------------------------------------------------------ updates
                SectionTitle(stringResource(R.string.section_updates))
                Column(Modifier.fillMaxWidth().glass()) {
                    Text(
                        stringResource(R.string.version_line, BuildConfig.VERSION_NAME, Updates.server(context).first),
                        color = Palette.muted, fontSize = 12.sp, modifier = Modifier.padding(16.dp),
                    )
                    SheetAction(Icons.Outlined.SystemUpdate, stringResource(R.string.check_updates)) { model.checkUpdate(manual = true) }
                    Toggle(stringResource(R.string.auto_updates), stringResource(R.string.auto_updates_hint), checkUpdates) {
                        checkUpdates = it
                        settings.checkUpdates = it
                        Updates.schedule(context, it)
                        if (it && Build.VERSION.SDK_INT >= 33) notifications.launch(Manifest.permission.POST_NOTIFICATIONS)
                    }
                }

                Text(
                    "2FAuth Android ${BuildConfig.VERSION_NAME}" + (state.config?.appVersion?.takeIf { it.isNotEmpty() }?.let { " · server $it" } ?: ""),
                    color = Palette.muted, fontSize = 11.sp, textAlign = TextAlign.Center,
                    modifier = Modifier.padding(vertical = 24.dp).fillMaxWidth(),
                )
            }
        }
    }

    if (confirmSignOut) {
        AlertDialog(
            onDismissRequest = { confirmSignOut = false },
            title = { Text(stringResource(R.string.sign_out)) },
            text = { Text(stringResource(R.string.sign_out_keep)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmSignOut = false
                    model.signOut(keepSynced = true)
                }) { Text(stringResource(R.string.sign_out_keep_yes)) }
            },
            dismissButton = {
                Row {
                    TextButton(onClick = {
                        confirmSignOut = false
                        model.signOut(keepSynced = false)
                    }) { Text(stringResource(R.string.sign_out_remove), color = Palette.danger) }
                    TextButton(onClick = { confirmSignOut = false }) { Text(stringResource(R.string.cancel)) }
                }
            },
            containerColor = Palette.sheet,
        )
    }
}

@Composable
private fun styleLabel(style: CardStyle): String = stringResource(
    when (style) {
        CardStyle.Classic -> R.string.style_classic
        CardStyle.Mesh -> R.string.style_mesh
        CardStyle.Double -> R.string.style_double
        CardStyle.Pill -> R.string.style_pill
        CardStyle.Bento -> R.string.style_bento
    },
)

/** A miniature of a theme: its background, a card and its accent. */
@Composable
private fun ThemeSwatch(preset: ThemePreset, selected: Boolean, onClick: () -> Unit) {
    val scale by animateFloatAsState(if (selected) 1f else 0.94f, label = "swatch")
    val border by animateColorAsState(if (selected) Color(preset.accent) else Color(preset.border).copy(alpha = 0.3f), label = "border")
    Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.width(84.dp).scale(scale).clickable(onClick = onClick)) {
        Box(
            Modifier.size(84.dp, 64.dp).clip(RoundedCornerShape(14.dp))
                .background(Brush.linearGradient(listOf(Color(preset.bg), Color(preset.bg2))))
                .border(2.dp, border, RoundedCornerShape(14.dp))
                .padding(10.dp),
        ) {
            Box(Modifier.fillMaxWidth().height(26.dp).clip(RoundedCornerShape(7.dp)).background(Color(preset.panel).copy(alpha = 0.9f)).border(1.dp, Color(preset.border).copy(alpha = 0.35f), RoundedCornerShape(7.dp)))
            Box(Modifier.align(Alignment.BottomStart).size(26.dp, 5.dp).clip(RoundedCornerShape(3.dp)).background(Color(preset.accent)))
            Box(Modifier.align(Alignment.BottomEnd).size(10.dp).clip(CircleShape).background(Color(preset.accent2)))
        }
        Text(preset.name, fontSize = 11.sp, color = if (selected) Palette.text else Palette.muted, maxLines = 1, modifier = Modifier.padding(top = 4.dp))
    }
}

@Composable
private fun AccentDot(color: Color, selected: Boolean, auto: Boolean, onClick: () -> Unit) {
    val scale by animateFloatAsState(if (selected) 1.1f else 1f, label = "dot")
    Box(
        Modifier.size(34.dp).scale(scale).clip(CircleShape).background(color)
            .border(if (selected) 3.dp else 1.dp, if (selected) Palette.text else Palette.border, CircleShape)
            .clickable(onClick = onClick),
        contentAlignment = Alignment.Center,
    ) {
        when {
            selected -> Icon(Icons.Outlined.Check, null, tint = if (color.red + color.green + color.blue > 2f) Color.Black else Color.White, modifier = Modifier.size(16.dp))
            auto -> Text("A", fontSize = 11.sp, fontWeight = FontWeight.Bold, color = Color.Black.copy(alpha = 0.6f))
        }
    }
}

@Composable
private fun StyleChip(label: String, selected: Boolean, onClick: () -> Unit) {
    Text(
        label,
        fontSize = 13.sp,
        color = if (selected) Palette.accent else Palette.muted,
        modifier = Modifier
            .clip(RoundedCornerShape(50))
            .background(if (selected) Palette.accent.copy(alpha = 0.16f) else Palette.panel)
            .border(1.dp, if (selected) Palette.accent.copy(alpha = 0.5f) else Palette.border, RoundedCornerShape(50))
            .clickable(onClick = onClick)
            .padding(horizontal = 14.dp, vertical = 8.dp),
    )
}

/** A card with made-up data, redrawn live as the theme and style change. */
@Composable
private fun PreviewCard() {
    val now = System.currentTimeMillis()
    CodeCard(
        content = CardContent(
            title = "GitHub", subtitle = "octo@example.org", hue = 220f, initials = "GI", icon = null,
            code = "492039", nextCode = "118230", remaining = 30 - ((now / 1000) % 30).toInt(), period = 30,
            hotp = false, digits = 6, pill = "30s",
            badges = listOf(Badge(stringResource(R.string.badge_synced), Palette.accent2)),
        ),
        hidden = false, showNext = true, split = true, flash = 0, onTap = {}, onLongPress = {},
    )
}

@Composable
fun Toggle(title: String, hint: String, checked: Boolean, enabled: Boolean = true, onChange: (Boolean) -> Unit) {
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
fun Choice(title: String, value: String, options: List<Pair<String, String>>, onChange: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    Row(
        Modifier.fillMaxWidth().clickable { open = true }.padding(horizontal = 16.dp, vertical = 14.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(title, fontSize = 15.sp, modifier = Modifier.weight(1f))
        Column {
            Text(options.firstOrNull { it.first == value }?.second ?: value, color = Palette.accent, fontSize = 14.sp)
            DropdownMenu(expanded = open, onDismissRequest = { open = false }, containerColor = Palette.sheet) {
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

const val LOCK_AUTHENTICATORS = androidx.biometric.BiometricManager.Authenticators.BIOMETRIC_WEAK or
    androidx.biometric.BiometricManager.Authenticators.DEVICE_CREDENTIAL
