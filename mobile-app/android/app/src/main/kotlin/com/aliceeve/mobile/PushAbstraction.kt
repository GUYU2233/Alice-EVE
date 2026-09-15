package com.aliceeve.mobile

/**
 * Stable provider identifiers used by the Flutter and server contracts.
 *
 * Provider SDKs are deliberately not referenced here. An application build can
 * add one SDK-specific adapter without changing the notification/router API.
 */
enum class PushProvider(
    val id: String,
    val displayName: String,
) {
    FCM("fcm", "Firebase Cloud Messaging"),
    XIAOMI_MI_PUSH("xiaomi", "Xiaomi Mi Push"),
    HUAWEI_PUSH_KIT("huawei", "Huawei Push Kit"),
    OPPO_PUSH("oppo", "OPPO PUSH"),
    VIVO_PUSH("vivo", "vivo Push");

    companion object {
        fun fromId(value: String?): PushProvider? = entries.firstOrNull { it.id == value }
    }
}

data class PushProviderCapabilities(
    val provider: PushProvider,
    val supportsDataMessages: Boolean,
    val supportsNotificationMessages: Boolean,
    val requiresVendorDevice: Boolean,
    val requiresVendorSdk: Boolean,
    val manifestPermissions: Set<String>,
    val runtimePermissions: Set<String>,
    val integrationNotes: String,
)

data class PushProviderConfiguration(
    /** True only when the provider SDK is linked in this application variant. */
    val sdkLinked: Boolean = false,
    /** True only when deployment supplied the provider's public app id/config. */
    val appConfigurationPresent: Boolean = false,
)

sealed interface AdapterInitialization {
    data class Ready(val provider: PushProvider) : AdapterInitialization
    data class NotConfigured(val provider: PushProvider, val reason: String) : AdapterInitialization
}

data class IncomingPushMessage(
    val provider: PushProvider,
    val title: String?,
    val body: String?,
    val data: Map<String, String> = emptyMap(),
)

data class RoutedNotification(
    val provider: PushProvider,
    val title: String,
    val body: String,
    val data: Map<String, String>,
    val route: NotificationRoute,
)

enum class NotificationRoute(val wireValue: String) {
    INTEL_ALERT("intel.alert"),
    MARKET_ALERT("market.alert"),
    CONVERSATION_UPDATE("conversation.update"),
    GENERAL("general");

    companion object {
        fun fromData(data: Map<String, String>): NotificationRoute = when (data["type"]) {
            "intel.alert" -> INTEL_ALERT
            "market.alert" -> MARKET_ALERT
            "conversation.update" -> CONVERSATION_UPDATE
            else -> GENERAL
        }
    }
}

/** Provider adapter contract. Implementations may be supplied by build flavors. */
interface PushProviderAdapter {
    val provider: PushProvider
    val capabilities: PushProviderCapabilities

    /**
     * Default implementation is intentionally not initialized. It prevents a
     * provider from being advertised as active until an SDK-specific adapter
     * is installed and deployment configuration is supplied.
     */
    fun initialize(configuration: PushProviderConfiguration): AdapterInitialization =
        AdapterInitialization.NotConfigured(
            provider,
            when {
                !configuration.sdkLinked -> "provider SDK is not linked in this build"
                !configuration.appConfigurationPresent -> "provider app configuration is not supplied"
                else -> "an SDK-specific adapter must implement initialization"
            },
        )

    fun normalizeMessage(message: IncomingPushMessage): RoutedNotification? =
        NotificationRouter.route(message)
}

private class DeclarativePushProviderAdapter(
    override val provider: PushProvider,
    override val capabilities: PushProviderCapabilities,
) : PushProviderAdapter

/** Single source of truth for supported providers and their Android needs. */
object PushProviderRegistry {
    private const val Internet = "android.permission.INTERNET"
    private const val PostNotifications = "android.permission.POST_NOTIFICATIONS"

    private val entries: Map<PushProvider, PushProviderCapabilities> = linkedMapOf(
        PushProvider.FCM to PushProviderCapabilities(
            provider = PushProvider.FCM,
            supportsDataMessages = true,
            supportsNotificationMessages = true,
            requiresVendorDevice = false,
            requiresVendorSdk = true,
            manifestPermissions = setOf(Internet),
            runtimePermissions = setOf(PostNotifications),
            integrationNotes = "Uses Firebase Messaging service/config supplied by the host app.",
        ),
        PushProvider.XIAOMI_MI_PUSH to PushProviderCapabilities(
            provider = PushProvider.XIAOMI_MI_PUSH,
            supportsDataMessages = true,
            supportsNotificationMessages = true,
            requiresVendorDevice = true,
            requiresVendorSdk = true,
            manifestPermissions = setOf(Internet),
            runtimePermissions = setOf(PostNotifications),
            integrationNotes = "Requires Mi Push app registration and MIUI background/auto-start policy review.",
        ),
        PushProvider.HUAWEI_PUSH_KIT to PushProviderCapabilities(
            provider = PushProvider.HUAWEI_PUSH_KIT,
            supportsDataMessages = true,
            supportsNotificationMessages = true,
            requiresVendorDevice = true,
            requiresVendorSdk = true,
            manifestPermissions = setOf(Internet),
            runtimePermissions = setOf(PostNotifications),
            integrationNotes = "Requires HMS Core availability and a deployment-supplied Push Kit app configuration.",
        ),
        PushProvider.OPPO_PUSH to PushProviderCapabilities(
            provider = PushProvider.OPPO_PUSH,
            supportsDataMessages = true,
            supportsNotificationMessages = true,
            requiresVendorDevice = true,
            requiresVendorSdk = true,
            manifestPermissions = setOf(Internet),
            runtimePermissions = setOf(PostNotifications),
            integrationNotes = "Requires Heytap/ColorOS push service configuration and channel policy review.",
        ),
        PushProvider.VIVO_PUSH to PushProviderCapabilities(
            provider = PushProvider.VIVO_PUSH,
            supportsDataMessages = true,
            supportsNotificationMessages = true,
            requiresVendorDevice = true,
            requiresVendorSdk = true,
            manifestPermissions = setOf(Internet),
            runtimePermissions = setOf(PostNotifications),
            integrationNotes = "Requires vivo push service configuration and vendor delivery policy review.",
        ),
    )

    fun allCapabilities(): List<PushProviderCapabilities> = entries.values.toList()

    fun capabilities(provider: PushProvider): PushProviderCapabilities =
        entries.getValue(provider)

    fun adapter(provider: PushProvider): PushProviderAdapter =
        DeclarativePushProviderAdapter(provider, capabilities(provider))
}

object NotificationRouter {
    fun route(message: IncomingPushMessage): RoutedNotification? {
        val title = message.title?.trim().orEmpty()
        val body = message.body?.trim().orEmpty()
        if (title.isEmpty() || body.isEmpty()) return null
        return RoutedNotification(
            provider = message.provider,
            title = title,
            body = body,
            data = message.data.toMap(),
            route = NotificationRoute.fromData(message.data),
        )
    }
}
