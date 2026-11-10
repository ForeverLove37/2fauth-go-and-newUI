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
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.ContentCopy
import androidx.compose.material.icons.outlined.DeleteOutline
import androidx.compose.material.icons.outlined.Folder
import androidx.compose.material.icons.outlined.FolderOff
import androidx.compose.material.icons.outlined.PushPin
import androidx.compose.material.icons.outlined.SkipNext
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.data.Account
import app.twofauth.android.data.Group
import app.twofauth.android.data.currentCode
import app.twofauth.android.data.identityHue
import app.twofauth.android.data.initials
import app.twofauth.android.data.label
import app.twofauth.android.data.nextCode
import app.twofauth.android.data.subLabel

/** What a long press on a card offers. */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AccountSheet(
    model: AppModel,
    account: Account,
    groups: List<Group>,
    now: Long,
    onCopy: (String) -> Unit,
    onDismiss: () -> Unit,
) {
    var choosingGroup by remember { mutableStateOf(false) }
    var confirmDelete by remember { mutableStateOf(false) }
    val owned = !account.isBorrowed

    BottomPanel(onDismiss) {
        Column(Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(bottom = 12.dp)) {
            Row(Modifier.padding(horizontal = 24.dp, vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                Avatar(account.initials(), identityColor(account.identityHue()), model.iconUrl(account.icon), 46.dp)
                Spacer(Modifier.width(14.dp))
                Column {
                    Text(account.label(), fontWeight = FontWeight.Bold, fontSize = 17.sp)
                    Text(account.subLabel(), color = Palette.muted, fontSize = 13.sp)
                    Text(
                        "${account.otpType.uppercase()} · ${account.digits} · ${account.algorithm.uppercase()}" +
                            (account.period?.let { " · ${it}s" } ?: ""),
                        color = Palette.muted,
                        fontSize = 11.sp,
                        fontFamily = Mono,
                    )
                }
            }
            Hairline()

            if (!choosingGroup) {
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
                    model.setFavorite(account, !account.isFavorite)
                    onDismiss()
                }
                if (owned && groups.isNotEmpty()) {
                    SheetAction(Icons.Outlined.Folder, stringResource(R.string.move_to_group)) { choosingGroup = true }
                }
                if (owned) {
                    SheetAction(Icons.Outlined.DeleteOutline, stringResource(R.string.delete), Palette.danger) { confirmDelete = true }
                }
            } else {
                Text(
                    stringResource(R.string.move_to_group),
                    color = Palette.muted,
                    fontSize = 12.sp,
                    modifier = Modifier.padding(start = 24.dp, top = 12.dp, bottom = 4.dp),
                )
                if (account.groupId != null) {
                    SheetAction(Icons.Outlined.FolderOff, stringResource(R.string.no_group)) {
                        model.moveToGroup(account, null)
                        onDismiss()
                    }
                }
                groups.forEach { group ->
                    val current = group.id == account.groupId
                    SheetAction(Icons.Outlined.Folder, group.name, if (current) Palette.accent else null) {
                        if (!current) model.moveToGroup(account, group)
                        onDismiss()
                    }
                }
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
                    model.delete(account)
                    onDismiss()
                }) { Text(stringResource(R.string.delete), color = Palette.danger) }
            },
            dismissButton = { TextButton(onClick = { confirmDelete = false }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.panelStrong,
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
                    .heightIn(max = 640.dp)
                    .clip(RoundedCornerShape(topStart = 24.dp, topEnd = 24.dp))
                    .background(Brush.verticalGradient(listOf(Color(0xFF111A2E), Color(0xFF0A0F1C))))
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
