package com.aliceeve.mobile

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Intent
import android.os.Build
import android.os.IBinder
import androidx.core.app.NotificationCompat

/**
 * User-visible foreground service used only when the user explicitly enables
 * background keep-alive. It does not bypass Android power management and does
 * not claim that FCM delivery can be guaranteed by a permanent connection.
 */
class KeepAliveService : Service() {
    companion object {
        const val ACTION_START = "com.aliceeve.mobile.action.KEEP_ALIVE_START"
        const val ACTION_STOP = "com.aliceeve.mobile.action.KEEP_ALIVE_STOP"
        const val CHANNEL_ID = "alice_eve_keep_alive"
        const val NOTIFICATION_ID = 7001

        fun start(context: android.content.Context) {
            val intent = Intent(context, KeepAliveService::class.java).setAction(ACTION_START)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) context.startForegroundService(intent) else context.startService(intent)
        }

        fun stop(context: android.content.Context) {
            context.stopService(Intent(context, KeepAliveService::class.java).setAction(ACTION_STOP))
        }
    }

    override fun onCreate() {
        super.onCreate()
        createChannel()
        startForeground(NOTIFICATION_ID, notification())
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopForeground(STOP_FOREGROUND_REMOVE)
            stopSelf()
            return START_NOT_STICKY
        }
        return START_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun createChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            getSystemService(NotificationManager::class.java)?.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "后台连接", NotificationManager.IMPORTANCE_LOW).apply {
                    description = "保持 Alice-EVE 后台连接状态"
                    setShowBadge(false)
                },
            )
        }
    }

    private fun notification(): Notification {
        val openIntent = PendingIntent.getActivity(
            this,
            7001,
            Intent(this, MainActivity::class.java).apply { flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP },
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.mipmap.ic_launcher)
            .setContentTitle("Alice-EVE 后台连接")
            .setContentText("后台保活已开启，FCM 仍由系统负责投递")
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setContentIntent(openIntent)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .build()
    }
}
