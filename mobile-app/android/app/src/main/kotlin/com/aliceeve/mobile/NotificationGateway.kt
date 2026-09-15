package com.aliceeve.mobile

/** Provider-neutral Android notification port.
 *
 * A FirebaseMessagingService adapter may forward background data messages to
 * this router. No Firebase project id, google-services file, or credential is
 * embedded in the application source.
 */
interface LocalNotificationPresenter {
    fun show(title: String, body: String, data: Map<String, String> = emptyMap())
}

class BackgroundMessageRouter(private val presenter: LocalNotificationPresenter) {
    /**
     * Normalizes all providers through the same route. The default is FCM for
     * backwards compatibility with the original Firebase-compatible port.
     */
    fun onMessage(
        data: Map<String, String>,
        title: String?,
        body: String?,
        provider: PushProvider = PushProvider.FCM,
    ) {
        val routed = NotificationRouter.route(IncomingPushMessage(provider, title, body, data)) ?: return
        presenter.show(routed.title, routed.body, routed.data)
    }
}
