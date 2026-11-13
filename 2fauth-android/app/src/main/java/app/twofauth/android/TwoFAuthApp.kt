package app.twofauth.android

import android.app.Application
import app.twofauth.android.data.IconCache
import app.twofauth.android.data.LocalSettings
import app.twofauth.android.data.LocalStore
import app.twofauth.android.data.Updates
import app.twofauth.android.data.Vault
import app.twofauth.android.ui.CardStyle
import app.twofauth.android.ui.Palette
import app.twofauth.android.ui.themePresets
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

/** What lives as long as the process: the vault, the settings and the icon cache. */
class TwoFAuthApp : Application() {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    lateinit var vault: Vault
        private set
    lateinit var settings: LocalSettings
        private set
    lateinit var store: LocalStore
        private set
    lateinit var icons: IconCache
        private set

    override fun onCreate() {
        super.onCreate()
        vault = Vault(this)
        settings = LocalSettings(this)
        store = LocalStore(this, vault)
        icons = IconCache(this)
        applyTheme()
        // Started now so that the codes are in memory by the time the first frame is drawn.
        scope.launch { store.load() }
        Updates.schedule(this, settings.checkUpdates)
    }

    fun applyTheme() {
        val preset = themePresets.firstOrNull { it.id == settings.themeId } ?: themePresets.first()
        val style = runCatching { CardStyle.valueOf(settings.cardStyle) }.getOrDefault(CardStyle.Classic)
        Palette.apply(preset, settings.accent, style, settings.animations)
    }
}
