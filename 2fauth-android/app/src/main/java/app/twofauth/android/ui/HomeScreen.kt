package app.twofauth.android.ui

import android.os.Build
import android.view.HapticFeedbackConstants
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.shape.CircleShape
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
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.itemsIndexed
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.CloudDone
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.Sync
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.pulltorefresh.PullToRefreshBox
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.rotate
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.CodeClipboard
import app.twofauth.android.data.Kind
import app.twofauth.android.data.LocalAccount
import app.twofauth.android.data.identityHueOf
import app.twofauth.android.data.initialsOf
import java.io.File
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** How long a hidden code stays readable after a tap, as on the web panel. */
const val REVEAL_MS = 5000L

private const val ALL = "\u0000all"
private const val PINNED = "\u0000pinned"
private const val PHONE = "\u0000phone"
private const val SYNCED = "\u0000synced"

/** The codes kept on the phone: phone-only and synced ones, computed here every second. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun HomeScreen(model: AppModel, state: UiState, accounts: List<LocalAccount>, onAdd: () -> Unit, onSettings: () -> Unit) {
    val context = LocalContext.current
    val view = LocalView.current
    val scope = rememberCoroutineScope()
    val settings = model.settings
    var query by rememberSaveable { mutableStateOf("") }
    var searching by rememberSaveable { mutableStateOf(false) }
    var filter by rememberSaveable { mutableStateOf(ALL) }
    var hideCodes by remember { mutableStateOf(settings.hideCodes) }
    var sheetFor by remember { mutableStateOf<String?>(null) }
    val revealed = remember { mutableStateMapOf<String, Long>() }
    val flashes = remember { mutableStateMapOf<String, Int>() }
    val hotpCodes = remember { mutableStateMapOf<String, String>() }
    val grid = rememberLazyGridState()

    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            value = System.currentTimeMillis()
            delay(1000 - value % 1000)
        }
    }

    BackHandler(enabled = searching) {
        searching = false
        query = ""
    }
    LaunchedEffect(query, filter) { grid.scrollToItem(0) }

    val groups = remember(accounts) { accounts.map { it.group }.filter { it.isNotBlank() }.distinct().sorted() }
    val visible = remember(accounts, query, filter) {
        val needle = query.trim().lowercase()
        accounts
            .filter {
                when (filter) {
                    ALL -> true
                    PINNED -> it.favorite
                    PHONE -> it.kind == Kind.LOCAL
                    SYNCED -> it.kind == Kind.SYNCED
                    else -> it.group == filter
                }
            }
            .filter { needle.isEmpty() || "${it.service} ${it.account}".lowercase().contains(needle) }
            .sortedWith(compareByDescending<LocalAccount> { it.favorite }.thenBy { it.createdAt })
    }

    fun copy(account: LocalAccount, code: String) {
        if (code.isEmpty()) return
        CodeClipboard.copy(context, code, settings.clipboardClear)
        revealed[account.id] = System.currentTimeMillis() + REVEAL_MS
        flashes[account.id] = (flashes[account.id] ?: 0) + 1
        view.performHapticFeedback(if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) HapticFeedbackConstants.CONFIRM else HapticFeedbackConstants.KEYBOARD_TAP)
        model.copied(account.label(), if (account.isHotp) null else account.remaining(System.currentTimeMillis()))
    }

    fun tap(account: LocalAccount) {
        if (account.isHotp) {
            scope.launch {
                val code = model.nextHotp(account)
                hotpCodes[account.id] = code
                copy(account, code)
            }
        } else {
            copy(account, account.codeAt(System.currentTimeMillis()))
        }
    }

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding()) {
            Row(Modifier.fillMaxWidth().padding(start = 16.dp, end = 4.dp, top = 8.dp, bottom = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                Box(Modifier.weight(1f)) { BrandHeader(stringResource(R.string.app_name), stringResource(R.string.tab_codes)) }
                if (state.user != null) SyncButton(state.syncing, state.syncError != null) { model.sync(manual = true) }
                IconButton(onClick = { searching = !searching; if (!searching) query = "" }) {
                    Icon(Icons.Outlined.Search, stringResource(R.string.search), tint = if (searching) Palette.accent else Palette.muted)
                }
                IconButton(onClick = { hideCodes = !hideCodes; settings.hideCodes = hideCodes }) {
                    Icon(
                        if (hideCodes) Icons.Outlined.Visibility else Icons.Outlined.VisibilityOff,
                        stringResource(if (hideCodes) R.string.show_all_codes else R.string.hide_all_codes),
                        tint = if (hideCodes) Palette.muted else Palette.accent,
                    )
                }
                if (model.lockEnabled()) {
                    IconButton(onClick = model::lockNow) { Icon(Icons.Outlined.Lock, stringResource(R.string.lock_now), tint = Palette.muted) }
                }
                IconButton(onClick = onSettings) { Icon(Icons.Outlined.Settings, stringResource(R.string.settings), tint = Palette.muted) }
            }

            AnimatedVisibility(searching) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    singleLine = true,
                    placeholder = { Text(stringResource(R.string.search_hint)) },
                    leadingIcon = { Icon(Icons.Outlined.Search, null, tint = Palette.muted) },
                    trailingIcon = { if (query.isNotEmpty()) IconButton(onClick = { query = "" }) { Icon(Icons.Outlined.Close, stringResource(R.string.clear)) } },
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                    shape = RoundedCornerShape(14.dp),
                    colors = fieldColors(),
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                )
            }

            val chips = buildList {
                add(ALL to stringResource(R.string.all))
                if (accounts.any { it.favorite }) add(PINNED to stringResource(R.string.pinned))
                if (accounts.any { it.kind == Kind.SYNCED }) {
                    add(PHONE to stringResource(R.string.kind_local))
                    add(SYNCED to stringResource(R.string.kind_synced))
                }
                groups.forEach { add(it to it) }
            }
            if (chips.size > 1) ChipRow(chips, filter) { filter = it }

            PullToRefreshBox(
                isRefreshing = state.syncing && state.user != null,
                onRefresh = { if (state.user != null) model.sync(manual = true) },
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    !model.store.loaded.collectAsStateValue() -> Unit
                    accounts.isEmpty() -> Welcome(signedIn = state.user != null, onAdd = onAdd, onSettings = onSettings)
                    visible.isEmpty() -> EmptyNote(stringResource(R.string.no_match))
                    else -> LazyVerticalGrid(
                        columns = GridCells.Adaptive(minSize = 330.dp),
                        state = grid,
                        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 120.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        itemsIndexed(visible, key = { _, it -> it.id }) { index, account ->
                            val icon by produceState<File?>(model.iconMemo[account.id], account.service, account.icon) {
                                value = model.iconFile(account)?.also { model.iconMemo[account.id] = it }
                            }
                            val hidden = hideCodes && (revealed[account.id] ?: 0L) < now
                            CodeCard(
                                content = localContent(account, now, if (settings.showIcons) icon else null, hotpCodes[account.id]),
                                hidden = hidden,
                                showNext = settings.showNext,
                                split = settings.splitDigits,
                                flash = flashes[account.id] ?: 0,
                                onTap = { tap(account) },
                                onLongPress = {
                                    view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
                                    sheetFor = account.id
                                },
                                modifier = Modifier.animateItem().entrance(index),
                            )
                        }
                        item(span = { GridItemSpan(maxLineSpan) }) {
                            Text(
                                stringResource(R.string.hint_gestures),
                                color = Palette.muted.copy(alpha = 0.7f),
                                fontSize = 12.sp,
                                textAlign = TextAlign.Center,
                                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                            )
                        }
                    }
                }
            }
        }
        AddButton(onAdd, Modifier.align(Alignment.BottomEnd))
    }

    sheetFor?.let { id ->
        accounts.firstOrNull { it.id == id }?.let { account ->
            LocalSheet(model, state, account, groups, now, onCopy = { copy(account, it) }, onDismiss = { sheetFor = null })
        } ?: run { sheetFor = null }
    }
}

@Composable
fun kotlinx.coroutines.flow.StateFlow<Boolean>.collectAsStateValue(): Boolean {
    val value by collectAsState()
    return value
}

/** What a phone code shows on its card. */
@Composable
fun localContent(account: LocalAccount, now: Long, icon: File?, hotpCode: String?): CardContent {
    val badges = buildList {
        if (account.favorite) add(Badge(stringResource(R.string.pinned), Palette.accent))
        if (account.kind == Kind.SYNCED) add(Badge(stringResource(R.string.badge_synced), Palette.accent2))
        if (account.detached) add(Badge(stringResource(R.string.badge_detached), Palette.warn))
        if (account.isHotp) add(Badge("#${account.counter}", Palette.warn))
    }
    val code = if (account.isHotp) hotpCode.orEmpty() else account.codeAt(now)
    val next = if (account.isHotp) "" else account.codeAt(now + account.period * 1000L)
    return CardContent(
        title = account.label(),
        subtitle = account.subLabel(),
        hue = identityHueOf(account.service, account.account, account.id),
        initials = initialsOf(account.service, account.account),
        icon = icon,
        code = code,
        nextCode = next,
        remaining = account.remaining(now),
        period = account.period,
        hotp = account.isHotp,
        digits = if (account.isSteam) 5 else account.digits,
        pill = when {
            account.isHotp -> "HOTP"
            account.isSteam -> "Steam"
            account.kind == Kind.SYNCED -> stringResource(R.string.badge_synced)
            else -> "${account.period}s"
        },
        badges = badges,
    )
}

/** Cards slide in one after the other the first time the list is shown. */
@Composable
fun Modifier.entrance(index: Int): Modifier {
    if (!Palette.motion || index > 12) return this
    val progress = remember { Animatable(0f) }
    LaunchedEffect(Unit) {
        delay(index * 40L)
        progress.animateTo(1f, tween(360))
    }
    return this.graphicsLayer {
        alpha = progress.value
        translationY = (1 - progress.value) * 40f
    }
}

@Composable
private fun SyncButton(syncing: Boolean, failed: Boolean, onClick: () -> Unit) {
    val angle by rememberInfiniteTransition(label = "sync").animateFloat(
        0f, -360f, infiniteRepeatable(tween(1100, easing = LinearEasing), RepeatMode.Restart), label = "angle",
    )
    IconButton(onClick = onClick) {
        Icon(
            when {
                syncing -> Icons.Outlined.Sync
                failed -> Icons.Outlined.CloudOff
                else -> Icons.Outlined.CloudDone
            },
            stringResource(R.string.sync_now),
            tint = if (failed) Palette.warn else if (syncing) Palette.accent else Palette.muted,
            modifier = if (syncing) Modifier.rotate(angle) else Modifier,
        )
    }
}

@Composable
fun ChipRow(chips: List<Pair<String, String>>, selected: String, onSelect: (String) -> Unit) {
    LazyRow(
        contentPadding = PaddingValues(horizontal = 16.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
        modifier = Modifier.padding(vertical = 4.dp),
    ) {
        items(chips, key = { it.first }) { (id, label) ->
            FilterChip(
                selected = selected == id,
                onClick = { onSelect(id) },
                label = { Text(label) },
                colors = FilterChipDefaults.filterChipColors(
                    containerColor = Palette.panel,
                    labelColor = Palette.muted,
                    selectedContainerColor = Palette.accent.copy(alpha = 0.16f),
                    selectedLabelColor = Palette.accent,
                ),
                border = FilterChipDefaults.filterChipBorder(
                    enabled = true,
                    selected = selected == id,
                    borderColor = Palette.border,
                    selectedBorderColor = Palette.accent.copy(alpha = 0.5f),
                ),
            )
        }
    }
}

/** The first screen of a new install: what the three kinds of codes are. */
@Composable
private fun Welcome(signedIn: Boolean, onAdd: () -> Unit, onSettings: () -> Unit) {
    val float by rememberInfiniteTransition(label = "float").animateFloat(
        -6f, 6f, infiniteRepeatable(tween(2600), RepeatMode.Reverse), label = "y",
    )
    LazyVerticalGrid(GridCells.Fixed(1), Modifier.fillMaxSize(), contentPadding = PaddingValues(24.dp)) {
        item {
            Column(Modifier.fillMaxWidth().padding(top = 32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Box(Modifier.graphicsLayer { translationY = if (Palette.motion) float else 0f }) { BrandMark(88.dp) }
                Spacer(Modifier.height(18.dp))
                Text(stringResource(R.string.welcome_title), fontSize = 21.sp, fontWeight = FontWeight.Bold, textAlign = TextAlign.Center)
                Spacer(Modifier.height(8.dp))
                Text(stringResource(R.string.welcome_text), color = Palette.muted, fontSize = 14.sp, textAlign = TextAlign.Center)
                Spacer(Modifier.height(22.dp))
                KindLine(stringResource(R.string.kind_local), stringResource(R.string.kind_local_hint), Palette.accent)
                KindLine(stringResource(R.string.kind_synced), stringResource(R.string.kind_synced_hint), Palette.accent2)
                KindLine(stringResource(R.string.kind_cloud), stringResource(R.string.kind_cloud_hint), Palette.accent3)
                Spacer(Modifier.height(18.dp))
                androidx.compose.material3.Button(onClick = onAdd, colors = primaryButton(), modifier = Modifier.fillMaxWidth().height(50.dp)) {
                    Text(stringResource(R.string.add_first))
                }
                if (!signedIn) {
                    TextButton(onClick = onSettings) { Text(stringResource(R.string.connect_server)) }
                }
            }
        }
    }
}

@Composable
private fun KindLine(title: String, text: String, tone: androidx.compose.ui.graphics.Color) {
    Row(Modifier.fillMaxWidth().padding(vertical = 5.dp).glass(SmallShape).padding(12.dp), verticalAlignment = Alignment.CenterVertically) {
        Box(Modifier.size(10.dp).clip(CircleShape).background(tone))
        Spacer(Modifier.width(12.dp))
        Column {
            Text(title, fontWeight = FontWeight.SemiBold, fontSize = 14.sp)
            Text(text, color = Palette.muted, fontSize = 12.5.sp)
        }
    }
}

@Composable
fun EmptyNote(text: String) {
    LazyVerticalGrid(GridCells.Fixed(1), Modifier.fillMaxSize(), contentPadding = PaddingValues(32.dp)) {
        item {
            Text(text, color = Palette.muted, fontSize = 15.sp, textAlign = TextAlign.Center, modifier = Modifier.fillMaxWidth().padding(top = 80.dp))
        }
    }
}

/** The add button; it grows in when the screen appears. */
@Composable
fun AddButton(onClick: () -> Unit, modifier: Modifier = Modifier) {
    val grow = remember { Animatable(if (Palette.motion) 0f else 1f) }
    LaunchedEffect(Unit) { grow.animateTo(1f, androidx.compose.animation.core.spring(dampingRatio = 0.55f, stiffness = 300f)) }
    androidx.compose.material3.FloatingActionButton(
        onClick = onClick,
        containerColor = Palette.accent,
        contentColor = Palette.accentContrast,
        shape = RoundedCornerShape(18.dp),
        modifier = modifier
            .padding(end = 20.dp, bottom = 20.dp)
            .graphicsLayer { scaleX = grow.value; scaleY = grow.value },
    ) {
        Icon(Icons.Outlined.Add, stringResource(R.string.add_account))
    }
}
