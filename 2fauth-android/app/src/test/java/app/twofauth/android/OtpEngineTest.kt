package app.twofauth.android

import app.twofauth.android.data.OtpAuth
import app.twofauth.android.data.OtpEngine
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** The codes computed on the phone must be the ones of the RFCs and of the server. */
class OtpEngineTest {
    // RFC 4226 appendix D: secret "12345678901234567890"
    private val rfcSecret = OtpAuth.base32Encode("12345678901234567890".toByteArray())

    @Test
    fun hotpMatchesRfc4226() {
        val expected = listOf("755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489")
        expected.forEachIndexed { counter, code -> assertEquals(code, OtpEngine.hotp(rfcSecret, counter.toLong())) }
    }

    @Test
    fun totpMatchesRfc6238() {
        val sha1 = rfcSecret
        val sha256 = OtpAuth.base32Encode("12345678901234567890123456789012".toByteArray())
        val sha512 = OtpAuth.base32Encode("1234567890123456789012345678901234567890123456789012345678901234".toByteArray())
        val vectors = listOf(
            Triple(59L, "sha1", "94287082"), Triple(1111111109L, "sha1", "07081804"), Triple(20000000000L, "sha1", "65353130"),
            Triple(59L, "sha256", "46119246"), Triple(1234567890L, "sha256", "91819424"),
            Triple(59L, "sha512", "90693936"), Triple(2000000000L, "sha512", "38618901"),
        )
        for ((time, algorithm, code) in vectors) {
            val secret = when (algorithm) { "sha256" -> sha256; "sha512" -> sha512; else -> sha1 }
            assertEquals("$algorithm at $time", code, OtpEngine.totpAt(secret, time, 30, 8, algorithm))
        }
    }

    @Test
    fun steamUsesItsAlphabet() {
        val code = OtpEngine.totpAt("JBSWY3DPEHPK3PXP", 1794285520, 30, 5, "sha1", steam = true)
        assertEquals(5, code.length)
        assertTrue(code.all { it in "23456789BCDFGHJKMNPQRTVWXY" })
    }

    @Test
    fun base32IsTolerant() {
        assertEquals(OtpEngine.hotp("JBSWY3DPEHPK3PXP", 1), OtpEngine.hotp("jbsw y3dp ehpk 3pxp====", 1))
        assertFalse(OtpEngine.isValidSecret("NOT1BASE"))
        assertTrue(OtpEngine.isValidSecret("JBSWY3DPEHPK3PXP"))
    }
}
