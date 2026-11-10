package app.twofauth.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/** The tokens of the web panel (styles.css, default theme), one for one. */
object Palette {
    val bg = Color(0xFF05070F)
    val bg2 = Color(0xFF080D18)
    val panel = Color(0xB8101727)
    val panelStrong = Color(0xEB0B111E)
    val border = Color(0x2E78A0FF)
    val borderStrong = Color(0x6B9EC0FF)
    val text = Color(0xFFE8EEFC)
    val muted = Color(0xFF8F9DBB)
    val accent = Color(0xFF55E6C1)
    val accent2 = Color(0xFF6EA8FF)
    val accent3 = Color(0xFFB07CFF)
    val accentContrast = Color(0xFF04121A)
    val danger = Color(0xFFFF6B81)
    val warn = Color(0xFFFFC46B)
    val cardTop = Color(0xEB1E2C48)
    val cardBottom = Color(0xE6090E1A)
    val otpGlowSoft = Color(0x526EA8FF)
}

val Mono = FontFamily.Monospace

private val typography = Typography().let { base ->
    base.copy(
        titleLarge = base.titleLarge.copy(fontWeight = FontWeight.Bold, letterSpacing = 0.3.sp),
        titleMedium = base.titleMedium.copy(fontWeight = FontWeight.SemiBold),
        labelSmall = base.labelSmall.copy(letterSpacing = 1.2.sp, fontWeight = FontWeight.Bold),
    )
}

@Composable
fun TwoFAuthTheme(content: @Composable () -> Unit) {
    MaterialTheme(
        colorScheme = darkColorScheme(
            primary = Palette.accent,
            onPrimary = Palette.accentContrast,
            secondary = Palette.accent2,
            tertiary = Palette.accent3,
            background = Palette.bg,
            onBackground = Palette.text,
            surface = Palette.panelStrong,
            onSurface = Palette.text,
            surfaceVariant = Palette.panel,
            onSurfaceVariant = Palette.muted,
            surfaceContainer = Color(0xFF0E1526),
            surfaceContainerHigh = Color(0xFF121A2D),
            surfaceContainerHighest = Color(0xFF162036),
            surfaceContainerLow = Color(0xFF0B111E),
            outline = Palette.borderStrong,
            outlineVariant = Palette.border,
            error = Palette.danger,
        ),
        typography = typography,
    ) {
        // The screens draw their own backdrop, not a Surface: text must still default to light.
        CompositionLocalProvider(LocalContentColor provides Palette.text, content = content)
    }
}

/** The "minimal" backdrop of the web panel: a dark gradient lit by two soft glows. */
@Composable
fun Backdrop(modifier: Modifier = Modifier, content: @Composable BoxScope.() -> Unit) {
    Box(
        modifier
            .fillMaxSize()
            .background(Brush.linearGradient(listOf(Palette.bg, Palette.bg2)))
            .drawBehind {
                drawRect(
                    Brush.radialGradient(
                        listOf(Palette.accent2.copy(alpha = 0.16f), Color.Transparent),
                        center = Offset(size.width * 0.5f, -size.height * 0.1f),
                        radius = size.width * 0.9f,
                    ),
                )
                drawRect(
                    Brush.radialGradient(
                        listOf(Palette.accent.copy(alpha = 0.10f), Color.Transparent),
                        center = Offset(size.width * 0.9f, size.height * 1.1f),
                        radius = size.width * 0.8f,
                    ),
                )
            },
        content = content,
    )
}

val CodeStyle = TextStyle(fontFamily = Mono, fontWeight = FontWeight.SemiBold)
