package com.aliceeve.mobile

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Test

class PushAbstractionTest {
    @Test
    fun registryContainsAllSupportedProvidersWithoutSecrets() {
        assertEquals(
            setOf("fcm", "xiaomi", "huawei", "oppo", "vivo"),
            PushProviderRegistry.allCapabilities().map { it.provider.id }.toSet(),
        )
        assertFalse(PushProviderRegistry.adapter(PushProvider.FCM).initialize(PushProviderConfiguration()).let { it is AdapterInitialization.Ready })
    }

    @Test
    fun routerNormalizesCategoriesAndRejectsEmptyMessages() {
        val routed = NotificationRouter.route(
            IncomingPushMessage(
                provider = PushProvider.VIVO_PUSH,
                title = " Intel ",
                body = " Hostile ",
                data = mapOf("type" to "intel.alert"),
            ),
        )
        assertNotNull(routed)
        assertEquals(NotificationRoute.INTEL_ALERT, routed!!.route)
        assertEquals("Intel", routed.title)
        assertEquals(null, NotificationRouter.route(IncomingPushMessage(PushProvider.FCM, "", "body")))
    }
}
