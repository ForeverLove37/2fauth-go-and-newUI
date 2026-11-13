package app.twofauth.android.ui

import androidx.compose.animation.AnimatedContent
import androidx.compose.animation.core.Animatable
import androidx.compose.animation.core.LinearEasing
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.snap
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInVertically
import androidx.compose.animation.slideOutVertically
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.ExperimentalFoundationApi
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.outlined.TouchApp
import androidx.compose.material3.Icon
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.draw.drawWithCache
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.PointMode
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.TextUnit
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import app.twofauth.android.R
import coil.ImageLoader
import coil.compose.SubcomposeAsyncImage
import coil.imageLoader
import java.io.File

/** The image loader of the app: SVG logos, and the certificate trust of the server. */
val LocalIcons = staticCompositionLocalOf<ImageLoader?> { null }

val CardShape = RoundedCornerShape(18.dp)
val SmallShape = RoundedCornerShape(11.dp)

/** The 2FAuth logo: two ribbons around a star. */
@Composable
fun BrandMark(size: Dp = 36.dp) {
    Image(painterResource(R.drawable.logo), contentDescription = null, modifier = Modifier.size(size))
}

/** A glass panel: the card gradient, a hairline border and the light of the top edge. */
fun Modifier.glass(shape: RoundedCornerShape = CardShape, highlighted: Color? = null): Modifier = this
    .clip(shape)
    .background(Brush.linearGradient(listOf(Palette.cardTop, Palette.cardBottom), start = Offset.Zero, end = Offset(600f, 900f)))
    .border(1.dp, highlighted ?: Palette.border, shape)

/** The colour of a service, as on the web cards. */
fun identityColor(hue: Float): Color = Color.hsl(hue, 0.72f, 0.58f)

/** Logo of the service on a light tile, or its initials on its colour. */
@Composable
fun Avatar(initials: String, color: Color, icon: File?, size: Dp = 42.dp) {
    val shape = RoundedCornerShape(size * 0.27f)
    val fallback: @Composable () -> Unit = {
        Box(
            Modifier.size(size).clip(shape).background(Brush.linearGradient(listOf(color, lerpColor(color, Color.Black, 0.4f)))),
            contentAlignment = Alignment.Center,
        ) {
            Text(initials, color = Color.White, fontWeight = FontWeight.ExtraBold, fontSize = (size.value * 0.36f).sp)
        }
    }
    if (icon == null) {
        fallback()
        return
    }
    SubcomposeAsyncImage(
        model = icon,
        contentDescription = null,
        contentScale = ContentScale.Fit,
        imageLoader = LocalIcons.current ?: LocalContext.current.imageLoader,
        modifier = Modifier.size(size).clip(shape).background(Color.White.copy(alpha = 0.94f)).padding(size * 0.12f),
        error = { fallback() },
        loading = { fallback() },
    )
}

fun lerpColor(a: Color, b: Color, t: Float) = Color(
    red = a.red + (b.red - a.red) * t,
    green = a.green + (b.green - a.green) * t,
    blue = a.blue + (b.blue - a.blue) * t,
    alpha = a.alpha,
)

/** Countdown of a TOTP window, gliding between the seconds. It warms up when time runs out. */
@Composable
fun CountdownRing(remaining: Int, period: Int, size: Dp = 38.dp) {
    val target = if (period > 0) remaining.toFloat() / period else 0f
    // Down smoothly over each second; refill at once when a new window starts.
    val previous = remember { floatArrayOf(target) }
    val refill = target > previous[0]
    previous[0] = target
    val fraction by animateFloatAsState(
        target,
        animationSpec = if (refill || !Palette.motion) snap() else tween(1000, easing = LinearEasing),
        label = "ring",
    )
    val color = when {
        remaining <= 5 -> Palette.danger
        remaining <= 10 -> Palette.warn
        else -> Palette.accent
    }
    Box(Modifier.size(size), contentAlignment = Alignment.Center) {
        Canvas(Modifier.size(size)) {
            val stroke = 4.dp.toPx()
            val inset = stroke / 2
            val arcSize = Size(this.size.width - stroke, this.size.height - stroke)
            drawArc(Palette.border, 0f, 360f, false, Offset(inset, inset), arcSize, style = Stroke(stroke))
            drawArc(color, -90f, 360f * fraction, false, Offset(inset, inset), arcSize, style = Stroke(stroke, cap = StrokeCap.Round))
        }
        Text("$remaining", fontFamily = Mono, fontSize = 12.sp, fontWeight = FontWeight.SemiBold, color = Palette.muted)
    }
}

/** Digits that roll into place one by one when the code changes. */
@Composable
fun RollingCode(text: String, fontSize: TextUnit, color: Color, letterSpacing: TextUnit) {
    if (!Palette.motion) {
        Text(text, style = CodeStyle, fontSize = fontSize, letterSpacing = letterSpacing, color = color, maxLines = 1)
        return
    }
    Row {
        text.forEachIndexed { index, char ->
            AnimatedContent(
                targetState = char,
                transitionSpec = {
                    (slideInVertically(tween(260, delayMillis = index * 35)) { -it / 2 } + fadeIn(tween(260, delayMillis = index * 35)))
                        .togetherWith(slideOutVertically(tween(200)) { it / 2 } + fadeOut(tween(160)))
                },
                label = "digit",
            ) { shown ->
                Text(shown.toString(), style = CodeStyle, fontSize = fontSize, letterSpacing = letterSpacing, color = color, maxLines = 1)
            }
        }
    }
}

/** The small uppercase badges of a card: pinned, synced, HOTP counter. */
@Composable
fun Flag(text: String, tone: Color? = null) {
    Text(
        text.uppercase(),
        fontSize = 9.5.sp,
        letterSpacing = 0.7.sp,
        fontWeight = FontWeight.Bold,
        color = tone ?: Palette.muted,
        maxLines = 1,
        overflow = TextOverflow.Ellipsis,
        modifier = Modifier
            .clip(RoundedCornerShape(5.dp))
            .background(Palette.accent2.copy(alpha = 0.10f))
            .border(1.dp, (tone ?: Palette.border).copy(alpha = if (tone == null) 1f else 0.45f), RoundedCornerShape(5.dp))
            .padding(horizontal = 6.dp, vertical = 2.dp),
    )
}

data class Badge(val text: String, val tone: Color? = null)

/** Everything a card shows; the same card serves the phone codes and the cloud codes. */
data class CardContent(
    val title: String,
    val subtitle: String,
    val hue: Float,
    val initials: String,
    val icon: File?,
    val code: String,
    val nextCode: String,
    val remaining: Int,
    val period: Int,
    val hotp: Boolean,
    val digits: Int,
    val pill: String,
    val badges: List<Badge>,
)

/**
 * One account as a card, in the card style of the theme (the five styles of the web
 * panel). A tap copies, a long press opens the actions; [flash] changes on each copy.
 */
@OptIn(ExperimentalFoundationApi::class)
@Composable
fun CodeCard(
    content: CardContent,
    hidden: Boolean,
    showNext: Boolean,
    split: Boolean,
    flash: Int,
    onTap: () -> Unit,
    onLongPress: () -> Unit,
    modifier: Modifier = Modifier,
) {
    val color = identityColor(content.hue)
    val style = Palette.cardStyle
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val scale by animateFloatAsState(if (pressed && Palette.motion) 0.97f else 1f, spring(stiffness = 600f), label = "press")
    val glow = remember { Animatable(0f) }
    LaunchedEffect(flash) {
        if (flash > 0) {
            glow.snapTo(0.35f)
            glow.animateTo(0f, tween(650))
        }
    }

    val shape = CardShape
    val surface: Modifier = when (style) {
        CardStyle.Classic -> Modifier.glass(shape).identity(color, strip = true)
        CardStyle.Pill -> Modifier.glass(shape).identity(color, strip = false)
        CardStyle.Mesh -> Modifier.clip(shape).background(Palette.panelStrong).mesh(color).border(1.dp, lerpColor(Palette.border, color, 0.25f), shape)
        CardStyle.Double -> Modifier
            .clip(shape)
            .background(if (Palette.light) Color.White else Palette.panelStrong)
            .border(1.dp, Palette.borderStrong, shape)
            .padding(4.dp)
            .border(1.dp, color.copy(alpha = 0.45f), RoundedCornerShape(14.dp))
        CardStyle.Bento -> Modifier.clip(shape).background(Palette.panelStrong).border(1.dp, Palette.border, shape)
    }

    Box(
        modifier
            .fillMaxWidth()
            .scale(scale)
            .then(surface)
            .combinedClickable(interactionSource = interaction, indication = null, onClick = onTap, onLongClick = onLongPress)
            .semantics { contentDescription = "${content.title}, ${content.subtitle}" },
    ) {
        if (style == CardStyle.Bento) {
            BentoBody(content, color, hidden, showNext, split)
        } else {
            val inner = if (style == CardStyle.Double) Modifier.padding(start = 14.dp, end = 12.dp, top = 10.dp, bottom = 10.dp)
            else Modifier.padding(start = 20.dp, end = 16.dp, top = 14.dp, bottom = 14.dp)
            Column(inner) {
                Header(content, color, pill = style == CardStyle.Pill)
                Spacer(Modifier.height(12.dp))
                CodeRow(content, hidden, showNext, split)
            }
        }
        // the flash of a copy
        Box(Modifier.matchParentSize().background(Palette.accent.copy(alpha = glow.value)))
    }
}

@Composable
private fun Header(content: CardContent, color: Color, pill: Boolean) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Avatar(content.initials, color, content.icon)
        Spacer(Modifier.width(12.dp))
        Column(Modifier.weight(1f)) {
            Text(content.title, fontWeight = FontWeight.SemiBold, fontSize = 15.sp, color = Palette.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(content.subtitle, fontSize = 12.5.sp, color = Palette.muted, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
            content.badges.forEach { Flag(it.text, it.tone) }
            if (pill) Pill(content.pill, color)
        }
    }
}

/** The capsule of the "pill" style: a glowing dot in the colour of the service. */
@Composable
private fun Pill(text: String, color: Color) {
    Row(
        Modifier
            .clip(RoundedCornerShape(50))
            .background(color.copy(alpha = 0.16f))
            .border(1.dp, color.copy(alpha = 0.4f), RoundedCornerShape(50))
            .padding(horizontal = 9.dp, vertical = 3.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Box(Modifier.size(6.dp).clip(CircleShape).background(color))
        Spacer(Modifier.width(5.dp))
        Text(text.uppercase(), fontSize = 10.sp, fontWeight = FontWeight.Bold, letterSpacing = 0.6.sp, color = lerpColor(color, Palette.text, 0.25f))
    }
}

@Composable
private fun CodeRow(content: CardContent, hidden: Boolean, showNext: Boolean, split: Boolean) {
    Row(verticalAlignment = Alignment.Bottom) {
        Column(Modifier.weight(1f)) {
            CodeText(content, hidden, split)
            if (showNext && !hidden && !content.hotp && content.nextCode.isNotEmpty()) {
                Text(
                    "› " + formatGrouped(content.nextCode, split),
                    color = Palette.muted,
                    fontSize = 11.sp,
                    fontFamily = Mono,
                    modifier = Modifier.padding(top = 2.dp),
                )
            }
        }
        if (!content.hotp && content.period > 0) CountdownRing(content.remaining, content.period)
    }
}

@Composable
private fun CodeText(content: CardContent, hidden: Boolean, split: Boolean) {
    when {
        content.hotp && content.code.isEmpty() -> Row(verticalAlignment = Alignment.CenterVertically) {
            Icon(Icons.Outlined.TouchApp, null, tint = Palette.accent2, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(6.dp))
            Text(androidx.compose.ui.res.stringResource(R.string.tap_generate), color = Palette.accent2, fontSize = 14.sp)
        }
        hidden || content.code.isEmpty() -> Text(
            "●".repeat(content.digits.coerceIn(4, 10)),
            style = CodeStyle,
            fontSize = 22.sp,
            letterSpacing = 3.sp,
            color = Palette.muted,
        )
        else -> RollingCode(formatGrouped(content.code, split), 30.sp, Palette.text, 2.sp)
    }
}

/** The bento style: the card as a box of tiles with a fine grain. */
@Composable
private fun BentoBody(content: CardContent, color: Color, hidden: Boolean, showNext: Boolean, split: Boolean) {
    // No width here: each tile chooses its own (the header spans the card, the ring is square).
    val tile = Modifier
        .clip(SmallShape)
        .background(Palette.panel)
        .border(1.dp, Palette.border, SmallShape)
    Column(Modifier.grain().padding(8.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        Box(Modifier.fillMaxWidth().then(tile).drawBehind { drawRect(color, size = Size(3.dp.toPx(), size.height)) }.padding(start = 12.dp, end = 10.dp, top = 9.dp, bottom = 9.dp)) {
            Header(content, color, pill = false)
        }
        Row(Modifier.height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            Box(Modifier.weight(1f).fillMaxHeight().then(tile).padding(horizontal = 12.dp, vertical = 10.dp), contentAlignment = Alignment.CenterStart) {
                Column {
                    CodeText(content, hidden, split)
                    if (showNext && !hidden && !content.hotp && content.nextCode.isNotEmpty()) {
                        Text("› " + formatGrouped(content.nextCode, split), color = Palette.muted, fontSize = 11.sp, fontFamily = Mono)
                    }
                }
            }
            if (!content.hotp && content.period > 0) {
                Box(Modifier.width(64.dp).fillMaxHeight().then(tile), contentAlignment = Alignment.Center) {
                    CountdownRing(content.remaining, content.period)
                }
            }
        }
    }
}

/** "123456" → "123 456", "12345678" → "1234 5678". */
fun formatGrouped(code: String, split: Boolean): String {
    if (!split || code.length < 6 || code.any { !it.isLetterOrDigit() }) return code
    val half = (code.length + 1) / 2
    return code.substring(0, half) + " " + code.substring(half)
}

/** The identity strip and glow of the classic card. */
fun Modifier.identity(color: Color, strip: Boolean): Modifier = this.drawBehind {
    drawRect(
        Brush.radialGradient(
            listOf(color.copy(alpha = 0.26f), Color.Transparent),
            center = Offset(size.width * 0.9f, -size.height * 0.1f),
            radius = size.width * 0.55f,
        ),
    )
    if (strip) drawRect(color, size = Size(4.dp.toPx(), size.height))
}

/** Ambient top glow: light pooled along the top edge, in the colours of the service and theme. */
fun Modifier.mesh(color: Color): Modifier = this.drawBehind {
    drawRect(Brush.radialGradient(listOf(color.copy(alpha = 0.42f), Color.Transparent), Offset(size.width * 0.18f, -size.height * 0.2f), size.width * 0.75f))
    drawRect(Brush.radialGradient(listOf(Palette.accent3.copy(alpha = 0.30f), Color.Transparent), Offset(size.width * 0.82f, -size.height * 0.25f), size.width * 0.6f))
    drawRect(Brush.radialGradient(listOf(Palette.accent.copy(alpha = 0.20f), Color.Transparent), Offset(size.width * 0.5f, -size.height * 0.35f), size.width * 0.5f))
    drawRect(
        Brush.horizontalGradient(listOf(Color.Transparent, lerpColor(color, Color.White, 0.4f), Color.Transparent)),
        topLeft = Offset(size.width * 0.18f, 0f),
        size = Size(size.width * 0.64f, 1.dp.toPx()),
    )
}

/** A fine noise for the bento cards: the points are worked out once per size. */
fun Modifier.grain(): Modifier = this.drawWithCache {
    val step = 3.dp.toPx()
    val points = ArrayList<Offset>()
    var seed = 7
    var y = 0f
    while (y < size.height) {
        var x = 0f
        while (x < size.width) {
            seed = seed * 1103515245 + 12345
            if ((seed ushr 16) and 7 == 0) points += Offset(x, y)
            x += step
        }
        y += step
    }
    val dot = 1.dp.toPx()
    onDrawBehind { drawPoints(points, PointMode.Points, Color.White.copy(alpha = 0.05f), strokeWidth = dot) }
}

/** Title of a block of settings. */
@Composable
fun SectionTitle(text: String) {
    Text(
        text.uppercase(),
        color = Palette.muted,
        fontSize = 11.sp,
        letterSpacing = 1.4.sp,
        fontWeight = FontWeight.Bold,
        modifier = Modifier.padding(start = 4.dp, top = 18.dp, bottom = 8.dp),
    )
}

@Composable
fun BrandHeader(title: String, subtitle: String) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        BrandMark()
        Spacer(Modifier.width(10.dp))
        Column {
            Text(title, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = Palette.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(subtitle.uppercase(), fontSize = 10.sp, letterSpacing = 1.4.sp, color = Palette.muted, fontWeight = FontWeight.Medium)
        }
    }
}

@Composable
fun Hairline() {
    Box(Modifier.fillMaxWidth().height(1.dp).background(Palette.border))
}
