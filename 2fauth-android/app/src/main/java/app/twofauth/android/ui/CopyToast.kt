package app.twofauth.android.ui

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Check
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import kotlinx.coroutines.delay

/** What the toast says after a code was copied; [id] grows with every copy so the same code twice shows again. */
data class Copied(val id: Long, val title: String, val detail: String, val warn: Boolean = false)

private const val SHOW_MS = 1800L

/**
 * The confirmation that a code was copied: a pill that drops in from the top.
 * Our own, so it shows on every Android version and every vendor skin, whether
 * or not the system shows its clipboard preview.
 */
@Composable
fun CopyToast(copied: Copied?, modifier: Modifier = Modifier) {
    var shown by remember { mutableStateOf<Copied?>(null) }
    var visible by remember { mutableStateOf(false) }
    LaunchedEffect(copied?.id) {
        val next = copied ?: return@LaunchedEffect
        shown = next
        visible = true
        delay(SHOW_MS)
        visible = false
    }
    val motion = Palette.motion
    AnimatedVisibility(
        visible = visible,
        modifier = modifier.statusBarsPadding().padding(top = 10.dp),
        enter = if (motion) slideInVertically(spring(dampingRatio = 0.62f, stiffness = Spring.StiffnessMediumLow)) { -it * 2 } + fadeIn(tween(160)) else fadeIn(tween(1)),
        exit = if (motion) slideOutVertically(tween(220)) { -it * 2 } + fadeOut(tween(200)) else fadeOut(tween(1)),
    ) {
        val item = shown ?: return@AnimatedVisibility
        val tone = if (item.warn) Palette.warn else Palette.accent
        Row(
            Modifier
                .widthIn(max = 360.dp)
                .shadow(18.dp, RoundedCornerShape(50), ambientColor = Color.Black, spotColor = Color.Black)
                .clip(RoundedCornerShape(50))
                .background(Palette.sheet)
                .border(1.dp, tone.copy(alpha = 0.45f), RoundedCornerShape(50))
                .padding(start = 8.dp, end = 18.dp, top = 8.dp, bottom = 8.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Box(
                Modifier.size(28.dp).clip(CircleShape).background(tone.copy(alpha = 0.18f)),
                contentAlignment = Alignment.Center,
            ) {
                val pop = remember(item.id) { Animatable(if (motion) 0.2f else 1f) }
                LaunchedEffect(item.id) { pop.animateTo(1f, spring(dampingRatio = 0.4f, stiffness = Spring.StiffnessMedium)) }
                Icon(Icons.Rounded.Check, null, tint = tone, modifier = Modifier.size(18.dp).scale(pop.value))
            }
            Spacer(Modifier.width(10.dp))
            Column {
                Text(item.title, fontSize = 14.sp, fontWeight = FontWeight.SemiBold, color = Palette.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
                if (item.detail.isNotEmpty()) {
                    Text(item.detail, fontSize = 12.sp, color = if (item.warn) Palette.warn else Palette.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
                }
            }
        }
    }
}
