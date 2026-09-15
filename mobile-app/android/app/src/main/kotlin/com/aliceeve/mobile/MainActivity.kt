package com.aliceeve.mobile

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.pm.PackageManager
import android.os.Build
import com.google.firebase.messaging.FirebaseMessaging
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.EventChannel
import io.flutter.plugin.common.MethodChannel

object FcmTokenEvents {
    private var sink: EventChannel.EventSink? = null

    @Synchronized
    fun attach(next: EventChannel.EventSink?) {
        sink = next
    }

    fun emit(token: String) {
        if (token.isBlank()) return
        val next: EventChannel.EventSink?
        synchronized(this) { next = sink }
        runOnMainThread { next?.success(token) }
    }

    private fun runOnMainThread(block: () -> Unit) {
        android.os.Handler(android.os.Looper.getMainLooper()).post(block)
    }
}

class MainActivity : FlutterActivity() {
    private companion object {
        const val CHANNEL = "eve_assistant_mobile/push"
        const val TOKEN_EVENTS_CHANNEL = "eve_assistant_mobile/push_tokens"
        const val NOTIFICATION_PERMISSION_REQUEST = 4101
    }

    override fun configureFlutterEngine(flutterEngine: FlutterEngine) {
        super.configureFlutterEngine(flutterEngine)
        createNotificationChannel()
        EventChannel(flutterEngine.dartExecutor.binaryMessenger, TOKEN_EVENTS_CHANNEL).setStreamHandler(object : EventChannel.StreamHandler {
            override fun onListen(arguments: Any?, events: EventChannel.EventSink?) = FcmTokenEvents.attach(events)
            override fun onCancel(arguments: Any?) = FcmTokenEvents.attach(null)
        })
        MethodChannel(flutterEngine.dartExecutor.binaryMessenger, CHANNEL).setMethodCallHandler { call, result ->
            when (call.method) {
                "getProviderCapabilities" -> result.success(
                    PushProviderRegistry.allCapabilities().map { capability ->
                        mapOf(
                            "provider" to capability.provider.id,
                            "supportsDataMessages" to capability.supportsDataMessages,
                            "supportsNotificationMessages" to capability.supportsNotificationMessages,
                            "requiresVendorDevice" to capability.requiresVendorDevice,
                            "requiresVendorSdk" to capability.requiresVendorSdk,
                            "manifestPermissions" to capability.manifestPermissions.toList(),
                            "runtimePermissions" to capability.runtimePermissions.toList(),
                            "integrationNotes" to capability.integrationNotes,
                        )
                    },
                )
                "initializeProvider" -> {
                    val provider = PushProvider.fromId(call.argument<String>("provider"))
                    if (provider == null) {
                        result.error("UNKNOWN_PROVIDER", "Unsupported push provider", null)
                    } else if (provider == PushProvider.FCM) {
                        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                            result.success(mapOf("provider" to provider.id, "ready" to false, "reason" to "notification permission is not granted"))
                            return@setMethodCallHandler
                        }
                        FirebaseMessaging.getInstance().token
                            .addOnSuccessListener { token ->
                                result.success(mapOf("provider" to provider.id, "ready" to true, "token" to token))
                            }
                            .addOnFailureListener { error ->
                                result.success(mapOf("provider" to provider.id, "ready" to false, "reason" to (error.message ?: "FCM token unavailable")))
                            }
                    } else {
                        // Vendor SDKs remain optional build-flavor integrations.
                        result.success(
                            mapOf(
                                "provider" to provider.id,
                                "ready" to false,
                                "reason" to "provider SDK/configuration is not supplied by this build",
                            ),
                        )
                    }
                }
                "requestNotificationPermission" -> result.success(requestNotificationPermission())
                "getKeepAliveEnabled" -> result.success(getPreferences(0).getBoolean("keep_alive_enabled", false))
                "setKeepAliveEnabled" -> {
                    val enabled = call.argument<Boolean>("enabled") ?: false
                    getPreferences(0).edit().putBoolean("keep_alive_enabled", enabled).apply()
                    if (enabled) KeepAliveService.start(this) else KeepAliveService.stop(this)
                    result.success(enabled)
                }
                else -> result.notImplemented()
            }
        }
    }

    private fun createNotificationChannel() {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val channel = NotificationChannel("alice_eve_alerts", "Alice-EVE alerts", NotificationManager.IMPORTANCE_DEFAULT)
            getSystemService(NotificationManager::class.java)?.createNotificationChannel(channel)
        }
    }

    /** Returns current state; when needed, starts the Android 13+ prompt. */
    private fun requestNotificationPermission(): Boolean {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return true
        val granted = checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED
        if (!granted) requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), NOTIFICATION_PERMISSION_REQUEST)
        return granted
    }
}
