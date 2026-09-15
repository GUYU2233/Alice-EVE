package com.aliceeve.mobile

import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import androidx.core.app.NotificationCompat
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage

/**
 * FCM token/message bridge. The raw token is returned only through the
 * Flutter method channel registration boundary and is never persisted here.
 */
class AliceFirebaseMessagingService : FirebaseMessagingService() {
    @Suppress("DEPRECATION")
    override fun onNewToken(token: String) {
        // Send the raw token only over the in-process EventChannel. Flutter
        // forwards it to the authenticated server registration endpoint.
        FcmTokenEvents.emit(token)
    }

    override fun onMessageReceived(message: RemoteMessage) {
        val data = message.data
        val title = message.notification?.title ?: data["title"]
        val body = message.notification?.body ?: data["body"]
        val routed = NotificationRouter.route(IncomingPushMessage(PushProvider.FCM, title, body, data)) ?: return
        val manager = getSystemService(NotificationManager::class.java) ?: return
        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O && manager.getNotificationChannel("alice_eve_alerts") == null) {
            manager.createNotificationChannel(NotificationChannel("alice_eve_alerts", "Alice-EVE alerts", NotificationManager.IMPORTANCE_DEFAULT))
        }
        val intent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
            putExtra("notification_type", routed.route.wireValue)
        }
        val pending = PendingIntent.getActivity(this, 0, intent, PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE)
        val notification = NotificationCompat.Builder(this, "alice_eve_alerts")
            .setSmallIcon(com.aliceeve.mobile.R.mipmap.ic_launcher)
            .setContentTitle(routed.title)
            .setContentText(routed.body)
            .setAutoCancel(true)
            .setContentIntent(pending)
            .build()
        manager.notify((System.currentTimeMillis() and 0x7fffffff).toInt(), notification)
    }
}
