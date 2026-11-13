package app.twofauth.android.ui

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxScope
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Typography
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.drawBehind
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.lerp
import androidx.compose.ui.graphics.luminance
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.sp

/** One of the themes of the web panel (src/theme/presets.ts), colour for colour. */
data class ThemePreset(
    val id: String,
    val name: String,
    val light: Boolean,
    val bg: Long,
    val bg2: Long,
    val panel: Long,
    val panelAlpha: Float,
    val panelStrong: Long,
    val border: Long,
    val borderStrong: Long,
    val text: Long,
    val muted: Long,
    val accent: Long,
    val accent2: Long,
    val accent3: Long,
    val danger: Long,
    val warn: Long,
    val glowA: Long,
    val glowC: Long,
)

/** Generated from the web presets, so that both apps offer the same themes. */
val themePresets = listOf(
        ThemePreset("deep-space", "深空霓虹", false, 0xFF05070F, 0xFF080D18, 0xFF101727, 0.72f, 0xFF0B111E, 0xFF78A0FF, 0xFF9EC0FF, 0xFFE8EEFC, 0xFF8F9DBB, 0xFF55E6C1, 0xFF6EA8FF, 0xFFB07CFF, 0xFFFF6B81, 0xFFFFC46B, 0xFF6EA8FF, 0xFF55E6C1),
        ThemePreset("polar-night", "极夜蓝", false, 0xFF030814, 0xFF061225, 0xFF0B1830, 0.78f, 0xFF08101F, 0xFF4A74C9, 0xFF7597FF, 0xFFE4ECFF, 0xFF7F92B8, 0xFF4FC3FF, 0xFF6F8CFF, 0xFF8F6CFF, 0xFFFF6B81, 0xFFFFC46B, 0xFF3F7BFF, 0xFF4A63FF),
        ThemePreset("glacier", "冰川薄荷", false, 0xFF04120F, 0xFF06201B, 0xFF0A2320, 0.74f, 0xFF061916, 0xFF3F8F7D, 0xFF6FE0C2, 0xFFE6FFF7, 0xFF8BB8AB, 0xFF3EE6B0, 0xFF38BDF8, 0xFF7CF0D0, 0xFFFF6B81, 0xFFFFC46B, 0xFF22D3A5, 0xFFA7F3D0),
        ThemePreset("cyber-violet", "赛博紫", false, 0xFF0A0416, 0xFF140A26, 0xFF1A0E30, 0.76f, 0xFF120A22, 0xFF8B5CF6, 0xFFC4A1FF, 0xFFF4ECFF, 0xFFA292C4, 0xFFD946EF, 0xFF8B5CF6, 0xFFF472B6, 0xFFFF6B81, 0xFFFFC46B, 0xFFA855F7, 0xFF6366F1),
        ThemePreset("sunset-amber", "日落琥珀", false, 0xFF120805, 0xFF1F0F06, 0xFF2A1409, 0.76f, 0xFF1B0D06, 0xFFB4673A, 0xFFF0A672, 0xFFFFF2E6, 0xFFC1A084, 0xFFFFB45C, 0xFFFF7A59, 0xFFFFD166, 0xFFFF6B81, 0xFFFFC46B, 0xFFFB923C, 0xFFFBBF24),
        ThemePreset("forest-moss", "森林苔藓", false, 0xFF05100A, 0xFF0A1C12, 0xFF0C2418, 0.78f, 0xFF07170F, 0xFF3F7A52, 0xFF79D69A, 0xFFEAFCEF, 0xFF8FB39B, 0xFF7EE787, 0xFFA3E635, 0xFF5EEAD4, 0xFFFF6B81, 0xFFFFC46B, 0xFF4ADE80, 0xFF14B8A6),
        ThemePreset("rose-quartz", "玫瑰石英", false, 0xFF12060C, 0xFF1E0A14, 0xFF280F1A, 0.76f, 0xFF1A0810, 0xFFA34A63, 0xFFF096AC, 0xFFFFEEF3, 0xFFC49AA9, 0xFFFF9AB5, 0xFFF0ABFC, 0xFFFFC2D1, 0xFFFF6B81, 0xFFFFC46B, 0xFFFB7185, 0xFFFDA4AF),
        ThemePreset("vaporwave", "蒸汽波", false, 0xFF0B0320, 0xFF180A33, 0xFF1D0D3D, 0.72f, 0xFF120726, 0xFFFF5EC4, 0xFFFFA8E0, 0xFFF7E9FF, 0xFFB79AD6, 0xFF00F5D4, 0xFFFF2FB9, 0xFF7C4DFF, 0xFFFF6B81, 0xFFFFC46B, 0xFFFF2FB9, 0xFF7C4DFF),
        ThemePreset("abyss-teal", "深海青", false, 0xFF02090D, 0xFF04161D, 0xFF08222B, 0.8f, 0xFF04161D, 0xFF2A7286, 0xFF5FD4E8, 0xFFE2FBFF, 0xFF7FA8B5, 0xFF22D3EE, 0xFF38BDF8, 0xFF2DD4BF, 0xFFFF6B81, 0xFFFFC46B, 0xFF22D3EE, 0xFF14B8A6),
        ThemePreset("magma", "岩浆橙", false, 0xFF0D0403, 0xFF1A0805, 0xFF25100A, 0.78f, 0xFF170806, 0xFF9C3B1B, 0xFFFF8A4C, 0xFFFFF0E8, 0xFFB89584, 0xFFFF7A1A, 0xFFFFD166, 0xFFFF4D6D, 0xFFFF6B81, 0xFFFFC46B, 0xFFFF5F1F, 0xFFE11D48),
        ThemePreset("graphite", "石墨单色", false, 0xFF08090B, 0xFF101216, 0xFF16181D, 0.82f, 0xFF0D0F12, 0xFF4B5563, 0xFF94A3B8, 0xFFF1F5F9, 0xFF9AA4B2, 0xFFE2E8F0, 0xFF94A3B8, 0xFFCBD5E1, 0xFFF87171, 0xFFFBBF24, 0xFF94A3B8, 0xFF64748B),
        ThemePreset("paper", "极简白", true, 0xFFF7F9FD, 0xFFEAF0FA, 0xFFFFFFFF, 0.82f, 0xFFFFFFFF, 0xFF1E3C78, 0xFF2F5BA8, 0xFF0F172A, 0xFF5B6785, 0xFF0E9F7E, 0xFF2F6FD8, 0xFF7C5CFF, 0xFFFF6B81, 0xFFFFC46B, 0xFFA5C8FF, 0xFFC6F5E6),
        ThemePreset("morning-mist", "晨雾灰", true, 0xFFF2F4F8, 0xFFE2E7F0, 0xFFFFFFFF, 0.86f, 0xFFFFFFFF, 0xFF334155, 0xFF475569, 0xFF111827, 0xFF5F6B80, 0xFF2563A8, 0xFF4B6FD6, 0xFF7A63D6, 0xFFFF6B81, 0xFFFFC46B, 0xFFB8C7E0, 0xFFCFE7DF),
        ThemePreset("linen", "亚麻米色", true, 0xFFFBF7EF, 0xFFF2E9DA, 0xFFFFFDF8, 0.88f, 0xFFFFFDF8, 0xFF7A5C3E, 0xFF9C7550, 0xFF2B2118, 0xFF7A6A58, 0xFFB06A2C, 0xFF8F6B3F, 0xFF6F8F5F, 0xFFFF6B81, 0xFFFFC46B, 0xFFE8C99B, 0xFFCFE0C3),
        ThemePreset("mint-paper", "薄荷纸感", true, 0xFFF4FBF8, 0xFFE3F4EC, 0xFFFFFFFF, 0.88f, 0xFFFFFFFF, 0xFF2F6B57, 0xFF3F8B72, 0xFF10261F, 0xFF5C776D, 0xFF13967A, 0xFF2F7ED8, 0xFF6A9F3A, 0xFFFF6B81, 0xFFFFC46B, 0xFFA7E8CF, 0xFFE5F7C9),
)

/** The look of the account cards, the same five styles as on the web. */
enum class CardStyle { Classic, Mesh, Double, Pill, Bento }

/**
 * The colours of the current theme. They are snapshot state: reading one in a composable
 * subscribes it, so a change of theme or accent recolours every screen at once.
 */
object Palette {
    var light by mutableStateOf(false); private set
    var bg by mutableStateOf(Color(0xFF05070F)); private set
    var bg2 by mutableStateOf(Color(0xFF080D18)); private set
    var panel by mutableStateOf(Color(0xB8101727)); private set
    var panelStrong by mutableStateOf(Color(0xEB0B111E)); private set
    var sheet by mutableStateOf(Color(0xFF0E1526)); private set
    var border by mutableStateOf(Color(0x2E78A0FF)); private set
    var borderStrong by mutableStateOf(Color(0x6B9EC0FF)); private set
    var text by mutableStateOf(Color(0xFFE8EEFC)); private set
    var muted by mutableStateOf(Color(0xFF8F9DBB)); private set
    var accent by mutableStateOf(Color(0xFF55E6C1)); private set
    var accent2 by mutableStateOf(Color(0xFF6EA8FF)); private set
    var accent3 by mutableStateOf(Color(0xFFB07CFF)); private set
    var accentContrast by mutableStateOf(Color(0xFF04121A)); private set
    var danger by mutableStateOf(Color(0xFFFF6B81)); private set
    var warn by mutableStateOf(Color(0xFFFFC46B)); private set
    var cardTop by mutableStateOf(Color(0xEB1E2C48)); private set
    var cardBottom by mutableStateOf(Color(0xE6090E1A)); private set
    var glowA by mutableStateOf(Color(0xFF6EA8FF)); private set
    var glowC by mutableStateOf(Color(0xFF55E6C1)); private set
    var cardStyle by mutableStateOf(CardStyle.Classic); private set
    var motion by mutableStateOf(true); private set
    val otpGlowSoft: Color get() = accent2.copy(alpha = 0.32f)

    /** Applies a preset, with an optional accent of the user's choice (0 keeps the preset's). */
    fun apply(preset: ThemePreset, customAccent: Long, style: CardStyle, animations: Boolean) {
        light = preset.light
        bg = Color(preset.bg)
        bg2 = Color(preset.bg2)
        val panelColor = Color(preset.panel)
        panel = panelColor.copy(alpha = preset.panelAlpha)
        panelStrong = Color(preset.panelStrong).copy(alpha = if (light) 0.96f else 0.92f)
        sheet = lerp(Color(preset.panelStrong), Color(preset.bg2), 0.3f).copy(alpha = 1f)
        border = Color(preset.border).copy(alpha = if (light) 0.2f else 0.18f)
        borderStrong = Color(preset.borderStrong).copy(alpha = if (light) 0.45f else 0.42f)
        text = Color(preset.text)
        muted = Color(preset.muted)
        accent = if (customAccent != 0L) Color(customAccent) else Color(preset.accent)
        accent2 = Color(preset.accent2)
        accent3 = Color(preset.accent3)
        accentContrast = if (accent.luminance() > 0.45f) Color(0xFF04121A) else Color.White
        danger = Color(preset.danger)
        warn = Color(preset.warn)
        // The card gradient of the web panel: the panel lightened at the top, darkened below.
        cardTop = (if (light) lerp(panelColor, Color.White, 0.6f) else lerp(panelColor, Color.White, 0.12f)).copy(alpha = 0.94f)
        cardBottom = (if (light) lerp(panelColor, Color.White, 0.85f) else lerp(panelColor, Color.Black, 0.45f)).copy(alpha = 0.92f)
        glowA = Color(preset.glowA)
        glowC = Color(preset.glowC)
        cardStyle = style
        motion = animations
    }
}

/** Accents offered next to the one of each theme. */
val accentChoices = listOf(0L, 0xFF55E6C1, 0xFF6EA8FF, 0xFFB07CFF, 0xFFFF7AB6, 0xFFFFB454, 0xFFFF6B6B, 0xFF4ADE80, 0xFF22D3EE, 0xFFF5F5F5)

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
    val scheme = if (Palette.light) {
        lightColorScheme(
            primary = Palette.accent, onPrimary = Palette.accentContrast, secondary = Palette.accent2, tertiary = Palette.accent3,
            background = Palette.bg, onBackground = Palette.text, surface = Palette.sheet, onSurface = Palette.text,
            surfaceVariant = Palette.panel, onSurfaceVariant = Palette.muted,
            surfaceContainer = Palette.sheet, surfaceContainerHigh = Palette.sheet, surfaceContainerHighest = Palette.sheet,
            surfaceContainerLow = Palette.sheet, outline = Palette.borderStrong, outlineVariant = Palette.border, error = Palette.danger,
        )
    } else {
        darkColorScheme(
            primary = Palette.accent, onPrimary = Palette.accentContrast, secondary = Palette.accent2, tertiary = Palette.accent3,
            background = Palette.bg, onBackground = Palette.text, surface = Palette.sheet, onSurface = Palette.text,
            surfaceVariant = Palette.panel, onSurfaceVariant = Palette.muted,
            surfaceContainer = Palette.sheet, surfaceContainerHigh = Palette.sheet, surfaceContainerHighest = Palette.sheet,
            surfaceContainerLow = Palette.sheet, outline = Palette.borderStrong, outlineVariant = Palette.border, error = Palette.danger,
        )
    }
    MaterialTheme(colorScheme = scheme, typography = typography) {
        // The screens draw their own backdrop, not a Surface: text must still follow the theme.
        CompositionLocalProvider(LocalContentColor provides Palette.text, content = content)
    }
}

/** The "minimal" backdrop of the web panel: the theme's gradient lit by two soft glows. */
@Composable
fun Backdrop(modifier: Modifier = Modifier, content: @Composable BoxScope.() -> Unit) {
    val bg = Palette.bg
    val bg2 = Palette.bg2
    val glowA = Palette.glowA
    val glowC = Palette.glowC
    Box(
        modifier
            .fillMaxSize()
            .background(Brush.linearGradient(listOf(bg, bg2)))
            .drawBehind {
                drawRect(
                    Brush.radialGradient(
                        listOf(glowA.copy(alpha = 0.16f), Color.Transparent),
                        center = Offset(size.width * 0.5f, -size.height * 0.1f),
                        radius = size.width * 0.9f,
                    ),
                )
                drawRect(
                    Brush.radialGradient(
                        listOf(glowC.copy(alpha = 0.10f), Color.Transparent),
                        center = Offset(size.width * 0.9f, size.height * 1.1f),
                        radius = size.width * 0.8f,
                    ),
                )
            },
        content = content,
    )
}

val CodeStyle = TextStyle(fontFamily = Mono, fontWeight = FontWeight.SemiBold)
