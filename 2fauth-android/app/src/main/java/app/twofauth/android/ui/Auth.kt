package app.twofauth.android.ui

import androidx.compose.runtime.staticCompositionLocalOf

/** Asks the user to prove it is them: fingerprint, face or the screen lock of the phone. */
fun interface Authenticator {
    suspend fun verify(title: String): Boolean
}

val LocalAuthenticator = staticCompositionLocalOf<Authenticator> { Authenticator { false } }
