package app.twofauth.android.ui

import android.content.ClipboardManager
import android.content.Context
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
import androidx.compose.material.icons.outlined.QrCodeScanner
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
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
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.AppModel
import app.twofauth.android.R
import app.twofauth.android.UiState
import app.twofauth.android.data.Kind
import app.twofauth.android.data.OtpAuth
import app.twofauth.android.data.OtpEngine
import app.twofauth.android.data.ParsedAccount
import app.twofauth.android.data.QrDecoder
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

private enum class AddMode { Choose, Scan, Link, Manual }

/** Where the new codes go: the three kinds. */
private enum class Target { Phone, Synced, Cloud }

@Composable
fun AddScreen(model: AppModel, state: UiState, initialUri: String?, onDone: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val authenticator = LocalAuthenticator.current
    var mode by rememberSaveable { mutableStateOf(if (initialUri != null) AddMode.Link else AddMode.Choose) }
    var busy by remember { mutableStateOf(false) }
    var found by remember { mutableStateOf<List<ParsedAccount>?>(null) }
    var foreign by remember { mutableStateOf<String?>(null) }
    var link by rememberSaveable { mutableStateOf(initialUri.orEmpty()) }
    val signedIn = state.user != null
    val verifyTitle = stringResource(R.string.verify_sync)

    fun back() {
        if (mode == AddMode.Choose || initialUri != null) onDone() else mode = AddMode.Choose
    }
    BackHandler(enabled = mode != AddMode.Scan) { back() }

    fun inspect(text: String) {
        val value = text.trim()
        if (value.isEmpty()) return
        try {
            found = OtpAuth.parse(value)
        } catch (_: OtpAuth.FormatException) {
            // Aegis, 2FAS, Bitwarden… exports: only the server reads them, as cloud codes.
            if (signedIn) foreign = value else model.say(context.getString(R.string.not_otpauth))
        }
    }

    val picker = rememberLauncherForActivityResult(ActivityResultContracts.PickVisualMedia()) { uri ->
        if (uri == null) return@rememberLauncherForActivityResult
        scope.launch {
            busy = true
            val text = withContext(Dispatchers.Default) { QrDecoder.fromUri(context, uri) }
            busy = false
            if (text != null) inspect(text) else model.say(context.getString(R.string.no_qr_found))
        }
    }

    LaunchedEffect(initialUri) { if (initialUri != null) inspect(initialUri) }

    if (mode == AddMode.Scan) {
        ScannerScreen(
            onResult = { text ->
                mode = AddMode.Choose
                inspect(text)
            },
            onBack = { mode = AddMode.Choose },
        )
    } else {
        Backdrop {
            Column(Modifier.fillMaxSize().statusBarsPadding().navigationBarsPadding().imePadding()) {
                Row(Modifier.padding(4.dp), verticalAlignment = Alignment.CenterVertically) {
                    IconButton(onClick = ::back) { Icon(Icons.AutoMirrored.Outlined.ArrowBack, stringResource(R.string.back)) }
                    Text(stringResource(R.string.add_account), fontWeight = FontWeight.Bold, fontSize = 19.sp)
                    Spacer(Modifier.weight(1f))
                    if (busy) CircularProgressIndicator(Modifier.size(22.dp).padding(end = 4.dp), strokeWidth = 2.dp, color = Palette.accent)
                    Spacer(Modifier.width(16.dp))
                }
                Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(horizontal = 16.dp)) {
                    when (mode) {
                        AddMode.Choose, AddMode.Scan -> {
                            ChoiceCard(Icons.Outlined.QrCodeScanner, stringResource(R.string.scan_qr), stringResource(R.string.scan_qr_hint_local)) { mode = AddMode.Scan }
                            ChoiceCard(Icons.Outlined.Image, stringResource(R.string.from_image), stringResource(R.string.from_image_hint)) {
                                picker.launch(PickVisualMediaRequest(ActivityResultContracts.PickVisualMedia.ImageOnly))
                            }
                            ChoiceCard(Icons.Outlined.ContentPaste, stringResource(R.string.paste_link), stringResource(R.string.paste_link_hint)) {
                                if (link.isEmpty()) link = clipboardText(context).orEmpty().takeIf { it.startsWith("otpauth", true) }.orEmpty()
                                mode = AddMode.Link
                            }
                            ChoiceCard(Icons.Outlined.Edit, stringResource(R.string.manual), stringResource(R.string.manual_hint)) { mode = AddMode.Manual }
                            Text(
                                stringResource(R.string.add_privacy),
                                color = Palette.muted, fontSize = 12.sp,
                                modifier = Modifier.padding(horizontal = 6.dp, vertical = 14.dp),
                            )
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
                                textStyle = TextStyle(fontFamily = Mono, fontSize = 13.sp),
                                modifier = Modifier.fillMaxWidth(),
                            )
                            Text(stringResource(R.string.link_help), color = Palette.muted, fontSize = 12.sp, modifier = Modifier.padding(vertical = 8.dp))
                            Button(onClick = { inspect(link) }, enabled = link.isNotBlank(), colors = primaryButton(), modifier = Modifier.fillMaxWidth().height(50.dp)) {
                                Text(stringResource(R.string.continue_action))
                            }
                        }
                        AddMode.Manual -> ManualForm { found = listOf(it) }
                    }
                    Spacer(Modifier.height(24.dp))
                }
            }
        }
    }

    found?.let { list ->
        var target by remember {
            mutableStateOf(if (model.settings.defaultKind == Kind.SYNCED && signedIn) Target.Synced else Target.Phone)
        }
        val existing = list.count { model.store.hasSecret(it.secret) }
        AlertDialog(
            onDismissRequest = { found = null },
            title = { Text(if (list.size == 1) list[0].service.ifBlank { list[0].account } else stringResource(R.string.import_count, list.size)) },
            text = {
                Column {
                    list.take(6).forEach {
                        Text(
                            "${it.service.ifBlank { "—" }} · ${it.account}",
                            fontSize = 13.sp, color = Palette.muted,
                        )
                    }
                    if (list.size > 6) Text("…", color = Palette.muted)
                    if (existing > 0) Text(stringResource(R.string.already_here, existing), color = Palette.warn, fontSize = 12.sp, modifier = Modifier.padding(top = 6.dp))
                    Spacer(Modifier.height(14.dp))
                    Text(stringResource(R.string.save_as), fontSize = 12.sp, color = Palette.muted)
                    Spacer(Modifier.height(6.dp))
                    val options = buildList {
                        add(Target.Phone to stringResource(R.string.kind_local))
                        if (signedIn) {
                            add(Target.Synced to stringResource(R.string.kind_synced))
                            add(Target.Cloud to stringResource(R.string.kind_cloud))
                        }
                    }
                    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                        options.forEachIndexed { index, (value, text) ->
                            SegmentedButton(
                                selected = target == value,
                                onClick = { target = value },
                                shape = SegmentedButtonDefaults.itemShape(index, options.size),
                                colors = segmentColors(),
                            ) { Text(text, fontSize = 12.sp, maxLines = 1) }
                        }
                    }
                    Text(
                        stringResource(
                            when (target) {
                                Target.Phone -> R.string.kind_local_hint
                                Target.Synced -> R.string.kind_synced_hint
                                Target.Cloud -> R.string.kind_cloud_hint
                            },
                        ),
                        fontSize = 12.sp, color = Palette.muted, modifier = Modifier.padding(top = 8.dp),
                    )
                }
            },
            confirmButton = {
                TextButton(enabled = !busy, onClick = {
                    scope.launch {
                        if (target == Target.Synced && !(model.canVerifyUser() && authenticator.verify(verifyTitle))) {
                            if (!model.canVerifyUser()) model.say(context.getString(R.string.need_screen_lock))
                            return@launch
                        }
                        busy = true
                        val added = when (target) {
                            Target.Phone -> model.addParsed(list, Kind.LOCAL)
                            Target.Synced -> model.addParsed(list, Kind.SYNCED)
                            Target.Cloud -> model.addToCloud(list)
                        }
                        busy = false
                        found = null
                        model.say(context.getString(R.string.added_count, added))
                        if (added > 0) onDone()
                    }
                }) { Text(stringResource(R.string.add)) }
            },
            dismissButton = { TextButton(onClick = { found = null }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.sheet,
        )
    }

    foreign?.let { payload ->
        AlertDialog(
            onDismissRequest = { foreign = null },
            title = { Text(stringResource(R.string.import_title)) },
            text = { Text(stringResource(R.string.import_foreign)) },
            confirmButton = {
                TextButton(enabled = !busy, onClick = {
                    scope.launch {
                        busy = true
                        val result = model.importToServer(payload)
                        busy = false
                        foreign = null
                        if (result != null) {
                            model.say(context.getString(R.string.import_result, result.imported, result.duplicates, result.invalid))
                            if (result.imported > 0) onDone()
                        }
                    }
                }) { Text(stringResource(R.string.import_action)) }
            },
            dismissButton = { TextButton(onClick = { foreign = null }) { Text(stringResource(R.string.cancel)) } },
            containerColor = Palette.sheet,
        )
    }
}

@Composable
fun segmentColors() = SegmentedButtonDefaults.colors(
    activeContainerColor = Palette.accent.copy(alpha = 0.18f),
    activeContentColor = Palette.accent,
    inactiveContainerColor = Palette.panel,
    inactiveContentColor = Palette.muted,
    activeBorderColor = Palette.accent.copy(alpha = 0.5f),
    inactiveBorderColor = Palette.border,
)

@Composable
private fun ManualForm(onReady: (ParsedAccount) -> Unit) {
    var service by rememberSaveable { mutableStateOf("") }
    var account by rememberSaveable { mutableStateOf("") }
    var secret by rememberSaveable { mutableStateOf("") }
    var type by rememberSaveable { mutableStateOf("totp") }
    var digits by rememberSaveable { mutableStateOf("6") }
    var period by rememberSaveable { mutableStateOf("30") }
    var counter by rememberSaveable { mutableStateOf("0") }
    var algorithm by rememberSaveable { mutableStateOf("sha1") }
    var advanced by rememberSaveable { mutableStateOf(false) }
    val validSecret = secret.isEmpty() || OtpEngine.isValidSecret(secret)

    Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
        SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
            listOf("totp" to "TOTP", "hotp" to "HOTP", "steamtotp" to "Steam").forEachIndexed { index, (value, text) ->
                SegmentedButton(selected = type == value, onClick = { type = value }, shape = SegmentedButtonDefaults.itemShape(index, 3), colors = segmentColors()) { Text(text) }
            }
        }
        Field(service, { service = it }, stringResource(R.string.service), capitalize = true)
        Field(account, { account = it }, stringResource(R.string.account_name))
        Field(secret, { secret = it.uppercase().replace(" ", "") }, stringResource(R.string.secret), mono = true)
        if (!validSecret) Text(stringResource(R.string.secret_invalid), color = Palette.danger, fontSize = 12.sp)
        Text(
            stringResource(if (advanced) R.string.hide_advanced else R.string.show_advanced),
            color = Palette.accent2, fontSize = 13.sp,
            modifier = Modifier.clickable { advanced = !advanced }.padding(vertical = 4.dp),
        )
        if (advanced && type != "steamtotp") {
            Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Field(digits, { digits = it.filter(Char::isDigit) }, stringResource(R.string.digits), number = true, modifier = Modifier.weight(1f))
                if (type == "totp") Field(period, { period = it.filter(Char::isDigit) }, stringResource(R.string.period), number = true, modifier = Modifier.weight(1f))
                else Field(counter, { counter = it.filter(Char::isDigit) }, stringResource(R.string.counter), number = true, modifier = Modifier.weight(1f))
            }
            SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                listOf("sha1", "sha256", "sha512", "md5").forEachIndexed { index, value ->
                    SegmentedButton(selected = algorithm == value, onClick = { algorithm = value }, shape = SegmentedButtonDefaults.itemShape(index, 4), colors = segmentColors()) {
                        Text(value.uppercase(), fontSize = 12.sp)
                    }
                }
            }
        }
        Button(
            onClick = {
                val steam = type == "steamtotp"
                onReady(
                    ParsedAccount(
                        service = service.trim(), account = account.trim(), secret = secret.trim(), otpType = type,
                        digits = if (steam) 5 else digits.toIntOrNull()?.coerceIn(5, 10) ?: 6,
                        algorithm = if (steam) "sha1" else algorithm,
                        period = if (steam) 30 else period.toIntOrNull()?.takeIf { it > 0 } ?: 30,
                        counter = counter.toLongOrNull() ?: 0,
                    ),
                )
            },
            enabled = (account.isNotBlank() || service.isNotBlank()) && secret.isNotBlank() && validSecret,
            colors = primaryButton(),
            modifier = Modifier.fillMaxWidth().height(50.dp),
        ) { Text(stringResource(R.string.continue_action)) }
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
        textStyle = if (mono) TextStyle(fontFamily = Mono) else TextStyle.Default,
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
