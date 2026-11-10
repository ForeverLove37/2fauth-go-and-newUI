package app.twofauth.android.ui

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext
import coil.ImageLoader
import coil.compose.SubcomposeAsyncImage
import coil.imageLoader

/** The image loader of the signed-in server. */
val LocalIcons = staticCompositionLocalOf<ImageLoader?> { null }

val CardShape = RoundedCornerShape(18.dp)
val SmallShape = RoundedCornerShape(11.dp)

/** The conic "2F" tile of the web panel's top bar. */
@Composable
fun BrandMark(size: Dp = 36.dp) {
    Box(
        Modifier
            .size(size)
            .clip(RoundedCornerShape(size * 0.32f))
            .background(Brush.sweepGradient(listOf(Palette.accent, Palette.accent2, Palette.accent3, Palette.accent))),
        contentAlignment = Alignment.Center,
    ) {
        Text("2F", color = Palette.accentContrast, fontWeight = FontWeight.Black, fontSize = (size.value * 0.42f).sp)
    }
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
fun Avatar(initials: String, color: Color, iconUrl: String?, size: Dp = 42.dp) {
    val shape = RoundedCornerShape(11.dp)
    val fallback: @Composable () -> Unit = {
        Box(
            Modifier
                .size(size)
                .clip(shape)
                .background(Brush.linearGradient(listOf(color, lerp(color, Color.Black, 0.4f)))),
            contentAlignment = Alignment.Center,
        ) {
            Text(initials, color = Color.White, fontWeight = FontWeight.ExtraBold, fontSize = (size.value * 0.36f).sp)
        }
    }
    if (iconUrl == null) {
        fallback()
        return
    }
    SubcomposeAsyncImage(
        model = iconUrl,
        contentDescription = null,
        imageLoader = LocalIcons.current ?: LocalContext.current.imageLoader,
        contentScale = ContentScale.Fit,
        modifier = Modifier
            .size(size)
            .clip(shape)
            .background(Color.White.copy(alpha = 0.94f))
            .padding(5.dp),
        error = { fallback() },
        loading = { fallback() },
    )
}

private fun lerp(a: Color, b: Color, t: Float) = Color(
    red = a.red + (b.red - a.red) * t,
    green = a.green + (b.green - a.green) * t,
    blue = a.blue + (b.blue - a.blue) * t,
    alpha = a.alpha,
)

/** Countdown of a TOTP window. It warms up in the last seconds, when a copy may be too late. */
@Composable
fun CountdownRing(remaining: Int, period: Int, size: Dp = 38.dp) {
    val fraction = if (period > 0) remaining.toFloat() / period else 0f
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

/** The small uppercase badges of a card: pinned, shared, HOTP counter. */
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

/** Title and hint of a block of settings. */
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
        Spacer(Modifier.width(12.dp))
        Column {
            Text(title, fontWeight = FontWeight.Bold, fontSize = 17.sp, color = Palette.text, maxLines = 1, overflow = TextOverflow.Ellipsis)
            Text(subtitle.uppercase(), fontSize = 10.sp, letterSpacing = 1.4.sp, color = Palette.muted, fontWeight = FontWeight.Medium)
        }
    }
}

/** The identity strip and the glow of a card, drawn behind its content. */
fun Modifier.identity(color: Color): Modifier = this.drawBehind {
    drawRect(
        Brush.radialGradient(
            listOf(color.copy(alpha = 0.26f), Color.Transparent),
            center = Offset(size.width * 0.9f, -size.height * 0.1f),
            radius = size.width * 0.55f,
        ),
    )
    drawRect(color, size = Size(4.dp.toPx(), size.height))
}

@Composable
fun Hairline() {
    Box(Modifier.fillMaxWidth().height(1.dp).background(Palette.border))
}
