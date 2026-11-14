package app.twofauth.android.ui

import android.os.Build
import android.view.HapticFeedbackConstants
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.Account
import app.twofauth.android.data.CodeClipboard
import app.twofauth.android.data.currentCode
import app.twofauth.android.data.identityHueOf
import app.twofauth.android.data.initialsOf
import app.twofauth.android.data.label
import app.twofauth.android.data.nextCode
import app.twofauth.android.data.remainingSeconds
import app.twofauth.android.data.subLabel
import java.io.File
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

private const val CLOUD_ALL = -1L
private const val CLOUD_PINNED = -2L

/** Cloud codes: their secrets stay on the server, the phone only receives short-lived codes. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun CloudScreen(model: AppModel, state: UiState, onConnect: () -> Unit, onSettings: () -> Unit) {
    val context = LocalContext.current
    val view = LocalView.current
    val scope = rememberCoroutineScope()
    var filter by rememberSaveable { mutableStateOf(CLOUD_ALL) }
    var sheetFor by remember { mutableStateOf<Long?>(null) }
    val revealed = remember { mutableStateMapOf<Long, Long>() }
    val flashes = remember { mutableStateMapOf<Long, Int>() }
    val hide = model.settings.hideCodes

    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            value = System.currentTimeMillis()
            delay(1000 - value % 1000)
        }
    }
    LaunchedEffect(state.user) { if (state.user != null && state.cloud.isEmpty()) model.refreshCloud(initial = true) }

    fun copy(account: Account, code: String) {
        if (code.isEmpty()) return
        CodeClipboard.copy(context, code, model.settings.clipboardClear)
        revealed[account.id] = System.currentTimeMillis() + REVEAL_MS
        flashes[account.id] = (flashes[account.id] ?: 0) + 1
        view.performHapticFeedback(if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) HapticFeedbackConstants.CONFIRM else HapticFeedbackConstants.KEYBOARD_TAP)
        model.copied(account.service?.ifBlank { null } ?: account.account.orEmpty(), if (account.isHotp) null else account.remainingSeconds(System.currentTimeMillis()))
    }

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding()) {
            Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f)) {
                    BrandHeader(state.config?.appName ?: stringResource(R.string.app_name), stringResource(R.string.tab_cloud))
                }
                IconButton(onClick = onSettings) { Icon(Icons.Outlined.Settings, stringResource(R.string.settings), tint = Palette.muted) }
            }

            if (state.user == null) {
                CloudIntro(onConnect)
                return@Column
            }

            val chips = buildList {
                add(CLOUD_ALL.toString() to stringResource(R.string.all))
                if (state.cloud.any { it.isFavorite }) add(CLOUD_PINNED.toString() to stringResource(R.string.pinned))
                state.groups.filter { it.showInChips && state.cloud.any { a -> a.groupId == it.id } }.forEach { add(it.id.toString() to it.name) }
            }
            if (chips.size > 1) ChipRow(chips, filter.toString()) { filter = it.toLong() }

            state.cloudError?.let {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).glass(SmallShape, Palette.warn.copy(alpha = 0.4f)).padding(horizontal = 12.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(Icons.Outlined.CloudOff, null, tint = Palette.warn, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(it, fontSize = 13.sp, modifier = Modifier.weight(1f), maxLines = 2, overflow = TextOverflow.Ellipsis)
                    TextButton(onClick = { model.refreshCloud(manual = true) }) { Text(stringResource(R.string.retry)) }
                }
            }

            val visible = state.cloud
                .filter { filter == CLOUD_ALL || (filter == CLOUD_PINNED && it.isFavorite) || it.groupId == filter }
                .sortedByDescending { it.isFavorite }

            PullToRefreshBox(isRefreshing = state.cloudRefreshing, onRefresh = { model.refreshCloud(manual = true) }, modifier = Modifier.fillMaxSize()) {
                when {
                    state.cloudLoading -> EmptyNote(stringResource(R.string.loading))
                    visible.isEmpty() -> EmptyNote(stringResource(R.string.cloud_empty))
                    else -> LazyVerticalGrid(
                        columns = GridCells.Adaptive(minSize = 330.dp),
                        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 120.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        itemsIndexed(visible, key = { _, it -> it.id }) { index, account ->
                            val icon by produceState<File?>(model.iconMemo[account.id], account.icon) {
                                value = model.iconFile(account)?.also { model.iconMemo[account.id] = it }
                            }
                            CodeCard(
                                content = cloudContent(account, now, if (model.settings.showIcons) icon else null, state.hotp[account.id]?.password),
                                hidden = hide && (revealed[account.id] ?: 0L) < now,
                                showNext = model.settings.showNext,
                                split = model.settings.splitDigits,
                                flash = flashes[account.id] ?: 0,
                                onTap = {
                                    if (account.isHotp) scope.launch { model.cloudHotp(account)?.let { copy(account, it.password) } }
                                    else copy(account, account.currentCode(System.currentTimeMillis()))
                                },
                                onLongPress = {
                                    view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
                                    sheetFor = account.id
                                },
                                modifier = Modifier.animateItem().entrance(index),
                            )
                        }
                    }
                }
            }
        }
    }

    sheetFor?.let { id ->
        state.cloud.firstOrNull { it.id == id }?.let { account ->
            CloudSheet(model, account, now, onCopy = { copy(account, it) }, onDismiss = { sheetFor = null })
        }
    }
}

@Composable
private fun cloudContent(account: Account, now: Long, icon: File?, hotpCode: String?): CardContent {
    val badges = buildList {
        if (account.isFavorite) add(Badge(stringResource(R.string.pinned), Palette.accent))
        if (account.isBorrowed) add(Badge(stringResource(R.string.borrowed)))
        if (account.isHotp) add(Badge("#${account.counter ?: 0}", Palette.warn))
    }
    return CardContent(
        title = account.label(),
        subtitle = account.subLabel(),
        hue = identityHueOf(account.service, account.account, account.id),
        initials = initialsOf(account.service, account.account),
        icon = icon,
        code = if (account.isHotp) hotpCode.orEmpty() else account.currentCode(now),
        nextCode = account.nextCode(now),
        remaining = account.remainingSeconds(now),
        period = account.period ?: 0,
        hotp = account.isHotp,
        digits = account.digits,
        pill = stringResource(R.string.kind_cloud),
        badges = badges,
    )
}

@Composable
private fun CloudIntro(onConnect: () -> Unit) {
    Column(Modifier.fillMaxSize().padding(28.dp), horizontalAlignment = Alignment.CenterHorizontally) {
        Spacer(Modifier.height(60.dp))
        BrandMark(72.dp)
        Spacer(Modifier.height(18.dp))
        Text(stringResource(R.string.cloud_title), fontWeight = FontWeight.Bold, fontSize = 20.sp, textAlign = TextAlign.Center)
        Spacer(Modifier.height(8.dp))
        Text(stringResource(R.string.cloud_intro), color = Palette.muted, fontSize = 14.sp, textAlign = TextAlign.Center)
        Spacer(Modifier.height(22.dp))
        Button(onClick = onConnect, colors = primaryButton(), modifier = Modifier.fillMaxWidth().height(50.dp)) {
            Text(stringResource(R.string.connect_server))
        }
    }
}

@Composable
fun fieldColors() = androidx.compose.material3.OutlinedTextFieldDefaults.colors(
    focusedBorderColor = Palette.accent.copy(alpha = 0.7f),
    unfocusedBorderColor = Palette.border,
    focusedContainerColor = Palette.panelStrong,
    unfocusedContainerColor = Palette.panel,
    cursorColor = Palette.accent,
    focusedLabelColor = Palette.accent,
    unfocusedLabelColor = Palette.muted,
    focusedPlaceholderColor = Palette.muted,
    unfocusedPlaceholderColor = Palette.muted,
    focusedTextColor = Palette.text,
    unfocusedTextColor = Palette.text,
)
