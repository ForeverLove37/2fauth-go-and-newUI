package app.twofauth.android.ui

import androidx.activity.compose.BackHandler
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.MutableTransitionState
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.Cloud
import androidx.compose.material.icons.outlined.CloudUpload
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.PhoneAndroid
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material.icons.outlined.SkipNext
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.Account
import app.twofauth.android.data.Kind
import app.twofauth.android.data.LocalAccount
import app.twofauth.android.data.currentCode
import app.twofauth.android.data.identityHueOf
import app.twofauth.android.data.initialsOf
import app.twofauth.android.data.label
import app.twofauth.android.data.nextCode
import app.twofauth.android.data.subLabel
import kotlinx.coroutines.launch

private enum class Confirm { Delete, Sync, Local, Cloud }

/** What a long press on a phone code offers, including moving it between the three kinds. */
@Composable
fun LocalSheet(
    model: AppModel,
    state: UiState,
    account: LocalAccount,
    groups: List<String>,
    now: Long,
    onCopy: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    val authenticator = LocalAuthenticator.current
    val scope = rememberCoroutineScope()
    var editing by remember { mutableStateOf(false) }
    var confirm by remember { mutableStateOf<Confirm?>(null) }
    val signedIn = state.user != null
    val verifyTitle = stringResource(R.string.verify_sync)

    BottomPanel(onDismiss) {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(bottom = 12.dp)) {
            Row(Modifier.padding(horizontal = 24.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Avatar(initialsOf(account.service, account.account), identityColor(identityHueOf(account.service, account.account, account.id)), model.iconMemo[account.id], 46.dp)
                Spacer(Modifier.width(14.dp))
                Column(Modifier.weight(1f)) {
                    Text(account.label(), fontWeight = FontWeight.Bold, fontSize = 17.sp)
                    Text(account.subLabel(), color = Palette.muted, fontSize = 13.sp)
                    Text(
                        stringResource(if (account.kind == Kind.SYNCED) R.string.kind_synced else R.string.kind_local) +
                            " · ${account.otpType.uppercase()} · ${account.digits} · ${account.algorithm.uppercase()}",
                        color = Palette.muted, fontSize = 11.sp, fontFamily = Mono,
                    )
                }
            }
            if (account.detached) {
                Text(
                    stringResource(R.string.detached_explained),
                    color = Palette.warn, fontSize = 13.sp,
                    modifier = Modifier.padding(horizontal = 24.dp, vertical = 6.dp).fillMaxWidth()
                        .clip(SmallShape).background(Palette.warn.copy(alpha = 0.1f)).padding(10.dp),
                )
            }
            Hairline()

            if (editing) {
                EditFields(account, groups) { updated ->
                    model.edit(updated)
                    onDismiss()
                }
                return@Column
            }

            if (!account.isHotp) {
                SheetAction(Icons.Outlined.ContentCopy, stringResource(R.string.copy_code)) {
                    onCopy(account.codeAt(System.currentTimeMillis()))
                    onDismiss()
                }
                SheetAction(Icons.Outlined.SkipNext, stringResource(R.string.copy_next_code)) {
                    onCopy(account.codeAt(now + account.period * 1000L))
                    onDismiss()
                }
            }
            SheetAction(Icons.Outlined.PushPin, stringResource(if (account.favorite) R.string.unpin else R.string.pin)) {
                model.toggleFavorite(account)
                onDismiss()
            }
            SheetAction(Icons.Outlined.Edit, stringResource(R.string.edit)) { editing = true }
            when {
                account.kind == Kind.LOCAL && signedIn ->
                    SheetAction(Icons.Outlined.CloudUpload, stringResource(R.string.action_sync), Palette.accent2) { confirm = Confirm.Sync }
                account.kind == Kind.SYNCED && signedIn -> {
                    SheetAction(Icons.Outlined.PhoneAndroid, stringResource(R.string.action_local)) { confirm = Confirm.Local }
                    SheetAction(Icons.Outlined.Cloud, stringResource(R.string.action_cloud)) { confirm = Confirm.Cloud }
                }
            }
            if (account.detached) {
                SheetAction(Icons.Outlined.Folder, stringResource(R.string.keep_on_phone)) {
                    model.acknowledgeDetached(account)
                    onDismiss()
                }
            }
            SheetAction(Icons.Outlined.DeleteOutline, stringResource(R.string.delete), Palette.danger) { confirm = Confirm.Delete }
        }
    }

    confirm?.let { what ->
        val (title, text, action) = when (what) {
            Confirm.Delete -> Triple(
                stringResource(R.string.delete_title, account.label()),
                stringResource(if (account.kind == Kind.SYNCED) R.string.delete_synced_text else R.string.delete_local_text),
                stringResource(R.string.delete),
            )
            Confirm.Sync -> Triple(stringResource(R.string.action_sync), stringResource(R.string.sync_explained), stringResource(R.string.action_sync_short))
            Confirm.Local -> Triple(stringResource(R.string.action_local), stringResource(R.string.local_explained), stringResource(R.string.confirm))
            Confirm.Cloud -> Triple(stringResource(R.string.action_cloud), stringResource(R.string.cloud_explained), stringResource(R.string.confirm))
        }
        AlertDialog(
            onDismissRequest = { confirm = null },
            title = { Text(title) },
            text = { Text(text) },
            confirmButton = {
                TextButton(onClick = {
                    confirm = null
                    when (what) {
                        Confirm.Delete -> { model.delete(account); onDismiss() }
                        Confirm.Local -> { model.makeLocal(account); onDismiss() }
                        Confirm.Cloud -> { model.makeCloud(account); onDismiss() }
                        Confirm.Sync -> scope.launch {
                            // A secret leaves the phone only once its owner has proved it is them.
                            if (!model.canVerifyUser()) {
                                model.say(model.getApplication<android.app.Application>().getString(R.string.need_screen_lock))
                            } else if (authenticator.verify(verifyTitle)) {
                                model.makeSynced(account)
                                onDismiss()
                            }
                        }
                    }
                }) { Text(action, color = if (what == Confirm.Delete) Palette.danger else Palette.accent) }
            },
            dismissButton = { TextButton(onClick = { confirm = null }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.sheet,
        )
    }
}

@Composable
private fun EditFields(account: LocalAccount, groups: List<String>, onSave: (LocalAccount) -> Unit) {
    var service by remember { mutableStateOf(account.service) }
    var name by remember { mutableStateOf(account.account) }
    var group by remember { mutableStateOf(account.group) }
    Column(Modifier.padding(horizontal = 20.dp, vertical = 12.dp)) {
        Field(service, { service = it }, stringResource(R.string.service), capitalize = true)
        Spacer(Modifier.size(10.dp))
        Field(name, { name = it }, stringResource(R.string.account_name))
        Spacer(Modifier.size(10.dp))
        Field(group, { group = it }, stringResource(R.string.group))
        if (groups.isNotEmpty()) {
            Row(Modifier.padding(top = 8.dp)) {
                ChipRow(listOf("" to stringResource(R.string.no_group)) + groups.map { it to it }, group) { group = it }
            }
        }
        Button(
            onClick = { onSave(account.copy(service = service.trim(), account = name.trim(), group = group.trim())) },
            colors = primaryButton(),
            modifier = Modifier.fillMaxWidth().padding(top = 14.dp),
        ) { Text(stringResource(R.string.save)) }
    }
}

/** What a long press on a cloud code offers. */
@Composable
fun CloudSheet(model: AppModel, account: Account, now: Long, onCopy: (String) -> Unit, onDismiss: () -> Unit) {
    var confirmDelete by remember { mutableStateOf(false) }
    BottomPanel(onDismiss) {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(bottom = 12.dp)) {
            Row(Modifier.padding(horizontal = 24.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Avatar(initialsOf(account.service, account.account), identityColor(identityHueOf(account.service, account.account, account.id)), model.iconMemo[account.id], 46.dp)
                Spacer(Modifier.width(14.dp))
                Column {
                    Text(account.label(), fontWeight = FontWeight.Bold, fontSize = 17.sp)
                    Text(account.subLabel(), color = Palette.muted, fontSize = 13.sp)
                    Text(stringResource(R.string.kind_cloud), color = Palette.muted, fontSize = 11.sp, fontFamily = Mono)
                }
            }
            Text(
                stringResource(R.string.cloud_sheet_hint),
                color = Palette.muted, fontSize = 12.5.sp,
                modifier = Modifier.padding(horizontal = 24.dp, vertical = 4.dp),
            )
            Hairline()
            if (!account.isHotp) {
                SheetAction(Icons.Outlined.ContentCopy, stringResource(R.string.copy_code)) {
                    onCopy(account.currentCode(System.currentTimeMillis()))
                    onDismiss()
                }
                val next = account.nextCode(now)
                if (next.isNotEmpty()) {
                    SheetAction(Icons.Outlined.SkipNext, stringResource(R.string.copy_next_code)) {
                        onCopy(next)
                        onDismiss()
                    }
                }
            }
            SheetAction(Icons.Outlined.PushPin, stringResource(if (account.isFavorite) R.string.unpin else R.string.pin)) {
                model.setCloudFavorite(account, !account.isFavorite)
                onDismiss()
            }
            if (!account.isBorrowed) {
                SheetAction(Icons.Outlined.DeleteOutline, stringResource(R.string.delete), Palette.danger) { confirmDelete = true }
            }
        }
    }
    if (confirmDelete) {
        AlertDialog(
            onDismissRequest = { confirmDelete = false },
            title = { Text(stringResource(R.string.delete_title, account.label())) },
            text = { Text(stringResource(R.string.delete_text)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmDelete = false
                    model.deleteCloud(account)
                    onDismiss()
                }) { Text(stringResource(R.string.delete), color = Palette.danger) }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = false }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.sheet,
        )
    }
}

@Composable
fun SheetAction(icon: ImageVector, text: String, tint: Color? = null, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().clickable(onClick = onClick).padding(horizontal = 24.dp, vertical = 15.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = tint ?: Palette.muted, modifier = Modifier.size(22.dp))
        Spacer(Modifier.width(18.dp))
        Text(text, color = tint ?: Palette.text, fontSize = 15.sp)
    }
}

/**
 * A sheet drawn in the screen itself. Material's ModalBottomSheet opens a window of
 * its own, which some devices never draw: this one cannot fail that way.
 */
@Composable
fun BottomPanel(onDismiss: () -> Unit, content: @Composable () -> Unit) {
    val shown = remember { MutableTransitionState(false) }.apply { targetState = true }
    BackHandler(onBack = onDismiss)
    Box(Modifier.fillMaxSize()) {
        AnimatedVisibility(shown, enter = fadeIn(), exit = fadeOut()) {
            Box(
                Modifier.fillMaxSize().background(Color.Black.copy(alpha = 0.55f))
                    .clickable(interactionSource = remember { MutableInteractionSource() }, indication = null, onClick = onDismiss),
            )
        }
        AnimatedVisibility(
            shown,
            enter = slideInVertically { it },
            exit = slideOutVertically { it },
            modifier = Modifier.align(Alignment.BottomCenter),
        ) {
            Column(
                Modifier
                    .widthIn(max = 640.dp)
                    .fillMaxWidth()
                    .heightIn(max = 680.dp)
                    .clip(RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    .background(Brush.verticalGradient(listOf(Palette.sheet, lerpColor(Palette.sheet, Palette.bg, 0.5f).copy(alpha = 1f))))
                    .border(1.dp, Palette.border, RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    // Taps inside the sheet must not reach the scrim.
                    .clickable(interactionSource = remember { MutableInteractionSource() }, indication = null) {}
                    .navigationBarsPadding(),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                Box(Modifier.padding(vertical = 10.dp).size(width = 36.dp, height = 4.dp).clip(RoundedCornerShape(2.dp)).background(Palette.muted.copy(alpha = 0.5f)))
                content()
            }
        }
    }
}
