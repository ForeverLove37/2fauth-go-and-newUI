package app.twofauth.android.data

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import androidx.core.content.ContextCompat
import androidx.core.content.FileProvider
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import app.twofauth.android.BuildConfig
import app.twofauth.android.MainActivity
import app.twofauth.android.R
import java.io.File
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

/**
 * New versions of the app are published on the 2FAuth server (Settings → Android app on
 * the web). The app asks that server, notifies the user, downloads the package, checks
 * its SHA-256 and hands it to the system installer, which only accepts it over the
 * installed app when it is signed with the same key.
 */
object Updates {
    private const val CHANNEL = "updates"
    private const val WORK = "update-check"

    /** The server asked: the signed-in one, or the one this build was made for. */
    fun server(context: Context): Pair<String, String?> {
        val vault = Vault(context)
        val signedIn = vault.get("server")
        return if (signedIn != null) signedIn to vault.get("pin") else BuildConfig.UPDATE_SERVER to null
    }

    suspend fun latest(context: Context): AndroidRelease? {
        val (base, pin) = server(context)
        val release = runCatching { Api(base, pin).androidLatest() }.getOrNull() ?: return null
        return release.takeIf { it.versionCode > BuildConfig.VERSION_CODE }
    }

    /** Downloads and checks the package; returns the file ready to install. */
    suspend fun download(context: Context, release: AndroidRelease, progress: (Float) -> Unit): File {
        val (base, pin) = server(context)
        val dir = File(context.cacheDir, "updates").apply { mkdirs() }
        dir.listFiles()?.forEach { if (!it.name.contains("-${release.versionCode}.")) it.delete() }
        val file = File(dir, "2fauth-${release.versionCode}.apk")
        val api = Api(base, pin)
        // The URL is relative to the host and carries the sub-directory, which base has too.
        val basePath = java.net.URL(base).path.trimEnd('/')
        val path = release.url.let { if (it.startsWith("http")) java.net.URL(it).path else it }.removePrefix(basePath)
        api.download(path, file, progress)
        val digest = MessageDigest.getInstance("SHA-256").digest(file.readBytes()).joinToString("") { "%02x".format(it) }
        if (release.sha256.isNotEmpty() && !digest.equals(release.sha256, true)) {
            file.delete()
            throw SecurityException("The downloaded package does not match its published checksum")
        }
        return file
    }

    /** Opens the system installer, or first the permission to install from this app. */
    fun install(context: Context, file: File): Boolean {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !context.packageManager.canRequestPackageInstalls()) {
            context.startActivity(
                Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
            return false
        }
        val uri = FileProvider.getUriForFile(context, "${context.packageName}.files", file)
        context.startActivity(
            Intent(Intent.ACTION_VIEW)
                .setDataAndType(uri, "application/vnd.android.package-archive")
                .addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK),
        )
        return true
    }

    /** Checks twice a day in the background, on any network. */
    fun schedule(context: Context, enabled: Boolean) {
        val work = WorkManager.getInstance(context)
        if (!enabled) {
            work.cancelUniqueWork(WORK)
            return
        }
        val request = PeriodicWorkRequestBuilder<UpdateWorker>(12, TimeUnit.HOURS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .build()
        work.enqueueUniquePeriodicWork(WORK, ExistingPeriodicWorkPolicy.KEEP, request)
    }

    fun notify(context: Context, release: AndroidRelease) {
        val prefs = context.getSharedPreferences("updates", Context.MODE_PRIVATE)
        if (prefs.getLong("notified", 0) >= release.versionCode) return
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(context, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            return
        }
        val manager = context.getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(
            NotificationChannel(CHANNEL, context.getString(R.string.channel_updates), NotificationManager.IMPORTANCE_DEFAULT),
        )
        val open = PendingIntent.getActivity(
            context,
            0,
            Intent(context, MainActivity::class.java).putExtra(MainActivity.EXTRA_UPDATE, true).addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP),
            PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT,
        )
        val notification = NotificationCompat.Builder(context, CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(context.getString(R.string.update_available, release.versionName))
            .setContentText(release.notes.lineSequence().firstOrNull().orEmpty().ifBlank { context.getString(R.string.update_tap) })
            .setStyle(NotificationCompat.BigTextStyle().bigText(release.notes.ifBlank { context.getString(R.string.update_tap) }))
            .setContentIntent(open)
            .setAutoCancel(true)
            .build()
        runCatching { NotificationManagerCompat.from(context).notify(1, notification) }
        prefs.edit().putLong("notified", release.versionCode).apply()
    }
}

class UpdateWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        Updates.latest(applicationContext)?.let { Updates.notify(applicationContext, it) }
        return Result.success()
    }
}
