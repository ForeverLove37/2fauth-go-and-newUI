package app.twofauth.android.ui

import android.os.Build
import android.view.HapticFeedbackConstants
import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.animateColorAsState
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBars
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawing
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.windowInsetsPadding
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.grid.rememberLazyGridState
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Add
import androidx.compose.material.icons.outlined.Close
import androidx.compose.material.icons.outlined.CloudOff
import androidx.compose.material.icons.outlined.Lock
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material.icons.outlined.TouchApp
import androidx.compose.material.icons.outlined.Visibility
import androidx.compose.material.icons.outlined.VisibilityOff
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.FilterChipDefaults
import androidx.compose.material3.FloatingActionButton
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalView
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.Account
import app.twofauth.android.data.CodeClipboard
import app.twofauth.android.data.currentCode
import app.twofauth.android.data.formatCode
import app.twofauth.android.data.identityHue
import app.twofauth.android.data.initials
import app.twofauth.android.data.label
import app.twofauth.android.data.nextCode
import app.twofauth.android.data.remainingSeconds
import app.twofauth.android.data.subLabel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/** How long a hidden code stays readable after a tap, as on the web panel. */
private const val REVEAL_MS = 5000L

private const val FILTER_ALL = -1L
private const val FILTER_PINNED = -2L

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountsScreen(model: AppModel, state: UiState, onSettings: () -> Unit, onAdd: () -> Unit) {
    val context = LocalContext.current
    val view = LocalView.current
    val scope = rememberCoroutineScope()
    var query by rememberSaveable { mutableStateOf("") }
    var filter by rememberSaveable { mutableStateOf(FILTER_ALL) }
    var searching by rememberSaveable { mutableStateOf(false) }
    var sheetFor by remember { mutableStateOf<Account?>(null) }
    val revealed = remember { mutableStateMapOf<Long, Long>() }
    val grid = rememberLazyGridState()

    val user = state.user
    // Hidden unless the user chose otherwise, like the web panel.
    val dots = user?.preference("showOtpAsDot", true) ?: true
    val showNext = user?.preference("showNextOtp", true) ?: true
    val showIcons = user?.preference("showAccountsIcons", true) ?: true
    val split = model.settings.splitDigits

    val now by produceState(System.currentTimeMillis()) {
        while (true) {
            value = System.currentTimeMillis()
            delay(1000 - value % 1000)
        }
    }

    // A new search or filter starts from the top, not where the previous list was scrolled.
    LaunchedEffect(query, filter) { grid.scrollToItem(0) }

    val visible = remember(state.accounts, query, filter) {
        val needle = query.trim().lowercase()
        state.accounts
            .filter { account ->
                when (filter) {
                    FILTER_ALL -> true
                    FILTER_PINNED -> account.isFavorite
                    else -> account.groupId == filter
                }
            }
            .filter { needle.isEmpty() || "${it.service.orEmpty()} ${it.account.orEmpty()}".lowercase().contains(needle) }
            // Pinned accounts first; the server order otherwise.
            .sortedByDescending { it.isFavorite }
    }

    fun copy(account: Account, code: String) {
        if (code.isEmpty()) return
        CodeClipboard.copy(context, code, model.settings.clipboardClear)
        revealed[account.id] = System.currentTimeMillis() + REVEAL_MS
        view.performHapticFeedback(
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) HapticFeedbackConstants.CONFIRM else HapticFeedbackConstants.KEYBOARD_TAP,
        )
        val left = account.remainingSeconds(System.currentTimeMillis())
        // Android 13 and later show their own confirmation of a copy.
        val message = when {
            !account.isHotp && left in 1..5 -> context.getString(R.string.copied_expiring, left)
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU -> null
            else -> context.getString(R.string.copied)
        }
        if (message != null) model.say(message)
    }

    fun tap(account: Account) {
        if (account.isHotp) {
            scope.launch { model.hotp(account)?.let { copy(account, it.password) } }
        } else {
            copy(account, account.currentCode(System.currentTimeMillis()))
        }
    }

    BackHandler(enabled = searching) {
        searching = false
        query = ""
    }

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding()) {
            // top bar ---------------------------------------------------------
            Row(
                Modifier.fillMaxWidth().padding(start = 18.dp, end = 6.dp, top = 8.dp, bottom = 4.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Box(Modifier.weight(1f)) {
                    BrandHeader(state.config?.appName ?: "2FAuth", stringResource(R.string.tagline))
                }
                IconButton(onClick = { searching = !searching; if (!searching) query = "" }) {
                    Icon(Icons.Outlined.Search, stringResource(R.string.search), tint = if (searching) Palette.accent else Palette.muted)
                }
                IconButton(onClick = { model.setServerPreference("showOtpAsDot", !dots) }) {
                    Icon(
                        if (dots) Icons.Outlined.Visibility else Icons.Outlined.VisibilityOff,
                        stringResource(if (dots) R.string.show_all_codes else R.string.hide_all_codes),
                        tint = if (dots) Palette.muted else Palette.accent,
                    )
                }
                if (model.lockEnabled()) {
                    IconButton(onClick = model::lockNow) {
                        Icon(Icons.Outlined.Lock, stringResource(R.string.lock_now), tint = Palette.muted)
                    }
                }
                IconButton(onClick = onSettings) {
                    Icon(Icons.Outlined.Settings, stringResource(R.string.settings), tint = Palette.muted)
                }
            }

            AnimatedVisibility(searching) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    singleLine = true,
                    placeholder = { Text(stringResource(R.string.search_hint)) },
                    leadingIcon = { Icon(Icons.Outlined.Search, null, tint = Palette.muted) },
                    trailingIcon = {
                        if (query.isNotEmpty()) IconButton(onClick = { query = "" }) { Icon(Icons.Outlined.Close, stringResource(R.string.clear)) }
                    },
                    keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search),
                    shape = RoundedCornerShape(14.dp),
                    colors = fieldColors(),
                    modifier = Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp),
                )
            }

            val chips = remember(state.groups, state.accounts) {
                buildList {
                    add(FILTER_ALL to null)
                    if (state.accounts.any { it.isFavorite }) add(FILTER_PINNED to null)
                    state.groups.filter { it.showInChips }.forEach { add(it.id to it.name) }
                }
            }
            if (chips.size > 1) {
                LazyRow(
                    contentPadding = PaddingValues(horizontal = 16.dp),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    modifier = Modifier.padding(vertical = 4.dp),
                ) {
                    items(chips, key = { it.first }) { (id, name) ->
                        FilterChip(
                            selected = filter == id,
                            onClick = { filter = id },
                            label = {
                                Text(
                                    when (id) {
                                        FILTER_ALL -> stringResource(R.string.all)
                                        FILTER_PINNED -> stringResource(R.string.pinned)
                                        else -> name.orEmpty()
                                    },
                                )
                            },
                            colors = FilterChipDefaults.filterChipColors(
                                containerColor = Palette.panel,
                                labelColor = Palette.muted,
                                selectedContainerColor = Palette.accent.copy(alpha = 0.16f),
                                selectedLabelColor = Palette.accent,
                            ),
                            border = FilterChipDefaults.filterChipBorder(
                                enabled = true,
                                selected = filter == id,
                                borderColor = Palette.border,
                                selectedBorderColor = Palette.accent.copy(alpha = 0.5f),
                            ),
                        )
                    }
                }
            }

            if (state.error != null && state.accounts.isNotEmpty()) {
                Row(
                    Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp).glass(SmallShape, Palette.warn.copy(alpha = 0.4f))
                        .padding(horizontal = 12.dp, vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                ) {
                    Icon(Icons.Outlined.CloudOff, null, tint = Palette.warn, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(8.dp))
                    Text(state.error, color = Palette.text, fontSize = 13.sp, modifier = Modifier.weight(1f), maxLines = 2, overflow = TextOverflow.Ellipsis)
                    TextButton(onClick = { model.refresh(manual = true) }) { Text(stringResource(R.string.retry)) }
                }
            }

            PullToRefreshBox(
                isRefreshing = state.refreshing,
                onRefresh = { model.refresh(manual = true) },
                modifier = Modifier.fillMaxSize(),
            ) {
                when {
                    state.loading -> Placeholder(stringResource(R.string.loading), null)
                    state.accounts.isEmpty() && state.error != null -> Placeholder(state.error, stringResource(R.string.retry)) { model.refresh(manual = true) }
                    state.accounts.isEmpty() -> Placeholder(stringResource(R.string.no_accounts), stringResource(R.string.add_account), onAdd)
                    visible.isEmpty() -> Placeholder(stringResource(R.string.no_match), null)
                    else -> LazyVerticalGrid(
                        columns = GridCells.Adaptive(minSize = 330.dp),
                        state = grid,
                        contentPadding = PaddingValues(start = 16.dp, end = 16.dp, top = 8.dp, bottom = 104.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        horizontalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.fillMaxSize(),
                    ) {
                        items(visible, key = { it.id }) { account ->
                            val hidden = dots && (revealed[account.id] ?: 0L) < now
                            AccountCard(
                                account = account,
                                now = now,
                                hidden = hidden,
                                showNext = showNext,
                                split = split,
                                iconUrl = if (showIcons) model.iconUrl(account.icon) else null,
                                hotpCode = state.hotp[account.id]?.password,
                                onTap = { tap(account) },
                                onLongPress = {
                                    view.performHapticFeedback(HapticFeedbackConstants.LONG_PRESS)
                                    sheetFor = account
                                },
                            )
                        }
                        item(span = { GridItemSpan(maxLineSpan) }) {
                            Text(
                                stringResource(R.string.hint_gestures),
                                color = Palette.muted.copy(alpha = 0.7f),
                                fontSize = 12.sp,
                                modifier = Modifier.fillMaxWidth().padding(top = 8.dp),
                                textAlign = androidx.compose.ui.text.style.TextAlign.Center,
                            )
                        }
                    }
                }
            }
        }

        FloatingActionButton(
            onClick = onAdd,
            containerColor = Palette.accent,
            contentColor = Palette.accentContrast,
            shape = RoundedCornerShape(18.dp),
            modifier = Modifier.align(Alignment.BottomEnd).windowInsetsPadding(WindowInsets.navigationBars).padding(20.dp),
        ) {
            Icon(Icons.Outlined.Add, stringResource(R.string.add_account))
        }
    }

    sheetFor?.let { account ->
        AccountSheet(
            model = model,
            account = state.accounts.firstOrNull { it.id == account.id } ?: account,
            groups = state.groups,
            now = now,
            onCopy = { code -> copy(account, code) },
            onDismiss = { sheetFor = null },
        )
    }
}

@OptIn(ExperimentalFoundationApi::class)
@Composable
fun AccountCard(
    account: Account,
    now: Long,
    hidden: Boolean,
    showNext: Boolean,
    split: Boolean,
    iconUrl: String?,
    hotpCode: String?,
    onTap: () -> Unit,
    onLongPress: () -> Unit,
) {
    val color = identityColor(account.identityHue())
    val code = if (account.isHotp) hotpCode.orEmpty() else account.currentCode(now)
    val remaining = account.remainingSeconds(now)
    val digitsColor by animateColorAsState(if (hidden) Palette.muted else Palette.text, label = "digits")
    val tapHint = stringResource(if (account.isHotp) R.string.tap_generate else R.string.tap_copy)

    Column(
        Modifier
            .fillMaxWidth()
            .glass()
            .identity(color)
            .combinedClickable(onClick = onTap, onLongClick = onLongPress, onClickLabel = tapHint)
            .semantics { contentDescription = "${account.label()}, ${account.subLabel()}" }
            .padding(start = 20.dp, end = 16.dp, top = 14.dp, bottom = 14.dp),
    ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            Avatar(account.initials(), color, iconUrl)
            Spacer(Modifier.width(12.dp))
            Column(Modifier.weight(1f)) {
                Text(account.label(), fontWeight = FontWeight.SemiBold, fontSize = 15.sp, color = Palette.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text(account.subLabel(), fontSize = 12.5.sp, color = Palette.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
            FlagRow(account)
        }
        Spacer(Modifier.height(12.dp))
        Row(verticalAlignment = Alignment.Bottom) {
            Column(Modifier.weight(1f)) {
                val shown = when {
                    account.isHotp && code.isEmpty() -> null
                    hidden || code.isEmpty() -> "●".repeat(account.digits.coerceIn(4, 10))
                    else -> formatCode(code, split)
                }
                if (shown == null) {
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        Icon(Icons.Outlined.TouchApp, null, tint = Palette.accent2, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.width(6.dp))
                        Text(stringResource(R.string.tap_generate), color = Palette.accent2, fontSize = 14.sp)
                    }
                } else {
                    Text(
                        shown,
                        style = CodeStyle,
                        fontSize = if (hidden) 22.sp else 30.sp,
                        letterSpacing = if (hidden) 3.sp else 2.sp,
                        color = digitsColor,
                        maxLines = 1,
                    )
                }
                val next = account.nextCode(now)
                if (showNext && !hidden && !account.isHotp && next.isNotEmpty()) {
                    Text(
                        stringResource(R.string.next_code, formatCode(next, split)),
                        color = Palette.muted,
                        fontSize = 11.sp,
                        fontFamily = Mono,
                        modifier = Modifier.padding(top = 2.dp),
                    )
                }
            }
            if (!account.isHotp && account.period != null) {
                CountdownRing(remaining, account.period)
            }
        }
    }
}

@Composable
private fun FlagRow(account: Account) {
    Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        if (account.isFavorite) Flag(stringResource(R.string.pinned), Palette.accent)
        when {
            account.isBorrowed -> Flag(stringResource(R.string.borrowed))
            account.isSharedWithAll -> Flag(stringResource(R.string.everyone))
            account.isShared -> Flag(stringResource(R.string.shared))
        }
        if (account.isHotp) Flag("#${account.counter ?: 0}", Palette.warn)
    }
}

@Composable
private fun Placeholder(text: String, action: String?, onAction: () -> Unit = {}) {
    // A scrollable child keeps pull-to-refresh working on an empty list.
    LazyVerticalGrid(GridCells.Fixed(1), Modifier.fillMaxSize(), contentPadding = PaddingValues(32.dp)) {
        item {
            Column(Modifier.fillMaxWidth().padding(top = 80.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                BrandMark(56.dp)
                Spacer(Modifier.height(18.dp))
                Text(text, color = Palette.muted, fontSize = 15.sp, textAlign = androidx.compose.ui.text.style.TextAlign.Center)
                if (action != null) {
                    Spacer(Modifier.height(12.dp))
                    TextButton(onClick = onAction) { Text(action, color = Palette.accent) }
                }
            }
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
)
