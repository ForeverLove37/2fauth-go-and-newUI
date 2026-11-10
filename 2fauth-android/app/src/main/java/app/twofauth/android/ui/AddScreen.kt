package app.twofauth.android.ui

import android.content.ClipboardManager
import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
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
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ArrowBack
import androidx.compose.material.icons.outlined.ContentPaste
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.Image
import androidx.compose.material.icons.outlined.PhotoCamera
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.core.content.FileProvider
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.data.Account
import app.twofauth.android.data.Group
import app.twofauth.android.data.label
import java.io.ByteArrayOutputStream
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

private enum class AddMode { Choose, Link, Manual }

/** What was read from a QR code or a link, waiting for the user to confirm it. */
private sealed interface Found {
    data class Single(val uri: String, val account: Account) : Found
    data class Export(val payload: String) : Found
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun AddScreen(model: AppModel, groups: List<Group>, initialUri: String?, onDone: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var mode by rememberSaveable { mutableStateOf(if (initialUri != null) AddMode.Link else AddMode.Choose) }
    var busy by remember { mutableStateOf(false) }
    var found by remember { mutableStateOf<Found?>(null) }
    var link by rememberSaveable { mutableStateOf(initialUri.orEmpty()) }
    var groupId by rememberSaveable { mutableStateOf<Long?>(null) }

    BackHandler { if (mode == AddMode.Choose || initialUri != null) onDone() else mode = AddMode.Choose }

    fun inspect(text: String) {
        val value = text.trim()
        if (value.isEmpty()) return
        scope.launch {
            busy = true
            found = if (value.startsWith("otpauth://totp", true) || value.startsWith("otpauth://hotp", true)) {
                model.preview(value)?.let { Found.Single(value, it) }
            } else {
                Found.Export(value)
            }
            busy = false
        }
    }

    fun scan(uri: Uri?) {
        if (uri == null) return
        scope.launch {
            busy = true
            val jpeg = withContext(Dispatchers.IO) { shrink(context, uri) }
            val text = jpeg?.let { model.decodeQr(it) }
            busy = false
            if (text != null) inspect(text) else if (jpeg == null) model.say(context.getString(R.string.image_unreadable))
        }
    }

    val photo = remember { File(File(context.cacheDir, "scans").apply { mkdirs() }, "scan.jpg") }
    val photoUri = remember { FileProvider.getUriForFile(context, "${context.packageName}.files", photo) }
    val camera = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { taken ->
        if (taken) scan(photoUri)
    }
    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { scan(it) }

    LaunchedEffect(initialUri) { if (initialUri != null) inspect(initialUri) }

    Backdrop {
        Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().imePadding()) {
            Row(Modifier.padding(horizontal = 4.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
                IconButton(onClick = { if (mode == AddMode.Choose || initialUri != null) onDone() else mode = AddMode.Choose }) {
                    Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back))
                }
                Text(stringResource(R.string.add_account), fontWeight = FontWeight.Bold, fontSize = 19.sp)
                Spacer(Modifier.weight(1f))
                if (busy) CircularProgressIndicator(Modifier.size(22.dp).padding(end = 4.dp), strokeWidth = 2.dp, color = Palette.accent)
                Spacer(Modifier.width(16.dp))
            }
            Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
                when (mode) {
                    AddMode.Choose -> {
                        ChoiceCard(Icons.Outlined.PhotoCamera, stringResource(R.string.scan_qr), stringResource(R.string.scan_qr_hint)) {
                            camera.launch(photoUri)
                        }
                        ChoiceCard(Icons.Outlined.Image, stringResource(R.string.from_image), stringResource(R.string.from_image_hint)) {
                            picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                        }
                        ChoiceCard(Icons.Outlined.ContentPaste, stringResource(R.string.paste_link), stringResource(R.string.paste_link_hint)) {
                            if (link.isEmpty()) link = clipboardText(context).orEmpty().takeIf { it.startsWith("otpauth", true) }.orEmpty()
                            mode = AddMode.Link
                        }
                        ChoiceCard(Icons.Outlined.Edit, stringResource(R.string.manual), stringResource(R.string.manual_hint)) {
                            mode = AddMode.Manual
                        }
                    }
                    AddMode.Link -> {
                        OutlinedTextField(
                            value = link,
                            onValueChange = { link = it },
                            label = { Text(stringResource(R.string.link_label)) },
                            placeholder = { Text("otpauth://totp/…") },
                            minLines = 3,
                            colors = fieldColors(),
                            shape = RoundedCornerShape(14.dp),
                            textStyle = androidx.compose.ui.text.TextStyle(fontFamily = Mono, fontSize = 13.sp),
                            modifier = Modifier.fillMaxWidth(),
                        )
                        Text(stringResource(R.string.link_help), color = Palette.muted, fontSize = 12.sp, modifier = Modifier.padding(vertical = 8.dp))
                        Button(
                            onClick = { inspect(link) },
                            enabled = !busy && link.isNotBlank(),
                            colors = primaryButton(),
                            modifier = Modifier.fillMaxWidth().height(50.dp),
                        ) { Text(stringResource(R.string.continue_action)) }
                    }
                    AddMode.Manual -> ManualForm(model, groups, busy, onBusy = { busy = it }, onDone = onDone)
                }
                Spacer(Modifier.height(24.dp))
            }
        }
    }

    when (val item = found) {
        is Found.Single -> AlertDialog(
            onDismissRequest = { found = null },
            title = { Text(item.account.label()) },
            text = {
                Column {
                    Text(item.account.account.orEmpty(), color = Palette.muted)
                    Text(
                        "${item.account.otpType.uppercase()} · ${item.account.digits} · ${item.account.algorithm.uppercase()}" +
                            (item.account.period?.let { " · ${it}s" } ?: ""),
                        fontFamily = Mono,
                        fontSize = 12.sp,
                        color = Palette.muted,
                        modifier = Modifier.padding(top = 4.dp, bottom = 12.dp),
                    )
                    if (groups.isNotEmpty()) GroupPicker(groups, groupId) { groupId = it }
                }
            },
            confirmButton = {
                TextButton(enabled = !busy, onClick = {
                    scope.launch {
                        busy = true
                        val created = model.create(buildJsonObject {
                            put("uri", item.uri)
                            groupId?.let { put("group_id", it) }
                        })
                        busy = false
                        found = null
                        if (created != null) {
                            model.say(context.getString(R.string.added, created.label()))
                            onDone()
                        }
                    }
                }) { Text(stringResource(R.string.add)) }
            },
            dismissButton = { TextButton(onClick = { found = null }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.panelStrong,
        )
        is Found.Export -> AlertDialog(
            onDismissRequest = { found = null },
            title = { Text(stringResource(R.string.import_title)) },
            text = { Text(stringResource(R.string.import_text)) },
            confirmButton = {
                TextButton(enabled = !busy, onClick = {
                    scope.launch {
                        busy = true
                        val result = model.import(item.payload)
                        busy = false
                        found = null
                        if (result != null) {
                            model.say(context.getString(R.string.import_result, result.imported, result.duplicates, result.invalid))
                            if (result.imported > 0) onDone()
                        }
                    }
                }) { Text(stringResource(R.string.import_action)) }
            },
            dismissButton = { TextButton(onClick = { found = null }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.panelStrong,
        )
        null -> Unit
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ManualForm(model: AppModel, groups: List<Group>, busy: Boolean, onBusy: (Boolean) -> Unit, onDone: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var service by rememberSaveable { mutableStateOf("") }
    var account by rememberSaveable { mutableStateOf("") }
    var secret by rememberSaveable { mutableStateOf("") }
    var type by rememberSaveable { mutableStateOf("totp") }
    var digits by rememberSaveable { mutableStateOf("6") }
    var period by rememberSaveable { mutableStateOf("30") }
    var counter by rememberSaveable { mutableStateOf("0") }
    var algorithm by rememberSaveable { mutableStateOf("sha1") }
    var groupId by rememberSaveable { mutableStateOf<Long?>(null) }
    var advanced by rememberSaveable { mutableStateOf(false) }

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            listOf("totp" to "TOTP", "hotp" to "HOTP", "steamtotp" to "Steam").forEachIndexed { index, (value, text) ->
                SegmentedButton(
                    selected = type == value,
                    onClick = { type = value },
                    shape = SegmentedButtonDefaults.itemShape(index, 3),
                    colors = SegmentedButtonDefaults.colors(
                        activeContainerColor = Palette.accent.copy(alpha = 0.18f),
                        activeContentColor = Palette.accent,
                        inactiveContainerColor = Palette.panel,
                        inactiveContentColor = Palette.muted,
                        activeBorderColor = Palette.accent.copy(alpha = 0.5f),
                        inactiveBorderColor = Palette.border,
                    ),
                ) { Text(text) }
            }
        }
        Field(service, { service = it }, stringResource(R.string.service), capitalize = true)
        Field(account, { account = it }, stringResource(R.string.account_name))
        Field(secret, { secret = it.uppercase().replace(" ", "") }, stringResource(R.string.secret), mono = true)
        if (groups.isNotEmpty()) GroupPicker(groups, groupId) { groupId = it }

        Text(
            stringResource(if (advanced) R.string.hide_advanced else R.string.show_advanced),
            color = Palette.accent2,
            fontSize = 13.sp,
            modifier = Modifier.clickable { advanced = !advanced }.padding(vertical = 4.dp),
        )
        if (advanced && type != "steamtotp") {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Field(digits, { digits = it.filter(Char::isDigit) }, stringResource(R.string.digits), number = true, modifier = Modifier.weight(1f))
                if (type == "totp") {
                    Field(period, { period = it.filter(Char::isDigit) }, stringResource(R.string.period), number = true, modifier = Modifier.weight(1f))
                } else {
                    Field(counter, { counter = it.filter(Char::isDigit) }, stringResource(R.string.counter), number = true, modifier = Modifier.weight(1f))
                }
            }
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                listOf("sha1", "sha256", "sha512", "md5").forEachIndexed { index, value ->
                    SegmentedButton(
                        selected = algorithm == value,
                        onClick = { algorithm = value },
                        shape = SegmentedButtonDefaults.itemShape(index, 4),
                    ) { Text(value.uppercase(), fontSize = 12.sp) }
                }
            }
        }

        Button(
            onClick = {
                scope.launch {
                    onBusy(true)
                    val created = model.create(buildJsonObject {
                        put("otp_type", type)
                        put("service", service.trim())
                        put("account", account.trim())
                        put("secret", secret.trim())
                        if (type != "steamtotp") {
                            put("algorithm", algorithm)
                            digits.toIntOrNull()?.let { put("digits", it) }
                            if (type == "totp") period.toIntOrNull()?.let { put("period", it) } else counter.toLongOrNull()?.let { put("counter", it) }
                        }
                        groupId?.let { put("group_id", it) }
                    })
                    onBusy(false)
                    if (created != null) {
                        model.say(context.getString(R.string.added, created.label()))
                        onDone()
                    }
                }
            },
            enabled = !busy && account.isNotBlank() && secret.isNotBlank(),
            colors = primaryButton(),
            modifier = Modifier.fillMaxWidth().height(50.dp),
        ) { Text(stringResource(R.string.add)) }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun GroupPicker(groups: List<Group>, selected: Long?, onSelect: (Long?) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val none = stringResource(R.string.no_group)
    ExposedDropdownMenuBox(expanded = open, onExpandedChange = { open = it }) {
        OutlinedTextField(
            value = groups.firstOrNull { it.id == selected }?.name ?: none,
            onValueChange = {},
            readOnly = true,
            label = { Text(stringResource(R.string.group)) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(open) },
            colors = fieldColors(),
            shape = RoundedCornerShape(14.dp),
            modifier = Modifier.fillMaxWidth().menuAnchor(MenuAnchorType.PrimaryNotEditable),
        )
        ExposedDropdownMenu(expanded = open, onDismissRequest = { open = false }, containerColor = Palette.panelStrong) {
            DropdownMenuItem(text = { Text(none) }, onClick = { onSelect(null); open = false })
            groups.forEach { group ->
                DropdownMenuItem(text = { Text(group.name) }, onClick = { onSelect(group.id); open = false })
            }
        }
    }
}

@Composable
fun Field(
    value: String,
    onChange: (String) -> Unit,
    label: String,
    modifier: Modifier = Modifier.fillMaxWidth(),
    mono: Boolean = false,
    number: Boolean = false,
    capitalize: Boolean = false,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onChange,
        label = { Text(label) },
        singleLine = true,
        colors = fieldColors(),
        shape = RoundedCornerShape(14.dp),
        textStyle = if (mono) androidx.compose.ui.text.TextStyle(fontFamily = Mono) else androidx.compose.ui.text.TextStyle.Default,
        keyboardOptions = KeyboardOptions(
            keyboardType = if (number) KeyboardType.Number else if (mono) KeyboardType.Ascii else KeyboardType.Text,
            capitalization = if (mono) KeyboardCapitalization.Characters else if (capitalize) KeyboardCapitalization.Words else KeyboardCapitalization.None,
            autoCorrectEnabled = false,
        ),
        modifier = modifier,
    )
}

@Composable
private fun ChoiceCard(icon: ImageVector, title: String, hint: String, onClick: () -> Unit) {
    Row(
        Modifier.fillMaxWidth().padding(vertical = 6.dp).glass().clickable(onClick = onClick).padding(18.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Icon(icon, null, tint = Palette.accent, modifier = Modifier.size(28.dp))
        Spacer(Modifier.width(16.dp))
        Column {
            Text(title, fontWeight = FontWeight.SemiBold, fontSize = 15.sp)
            Text(hint, color = Palette.muted, fontSize = 12.5.sp)
        }
    }
}

@Composable
fun primaryButton() = ButtonDefaults.buttonColors(
    containerColor = Palette.accent,
    contentColor = Palette.accentContrast,
    disabledContainerColor = Palette.accent.copy(alpha = 0.25f),
    disabledContentColor = Palette.accentContrast.copy(alpha = 0.6f),
)

private fun clipboardText(context: Context): String? = runCatching {
    context.getSystemService(ClipboardManager::class.java)?.primaryClip?.getItemAt(0)?.coerceToText(context)?.toString()?.trim()
}.getOrNull()

/** A camera photo is several megabytes: a QR code reads as well from 1600 pixels. */
private fun shrink(context: Context, uri: Uri): ByteArray? = runCatching {
    val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
    context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, bounds) }
    var sample = 1
    while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= 1600) sample *= 2
    val bitmap = context.contentResolver.openInputStream(uri)?.use {
        BitmapFactory.decodeStream(it, null, BitmapFactory.Options().apply { inSampleSize = sample })
    } ?: return null
    ByteArrayOutputStream().use { out ->
        bitmap.compress(Bitmap.CompressFormat.JPEG, 92, out)
        bitmap.recycle()
        out.toByteArray()
    }
}.getOrNull().also {
    // The photo of a secret does not stay on the phone.
    if (uri.authority == "${context.packageName}.files") runCatching { context.contentResolver.delete(uri, null, null) }
}
