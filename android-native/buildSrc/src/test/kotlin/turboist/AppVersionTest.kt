package turboist

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith
import kotlin.test.assertTrue

class AppVersionTest {
    @Test
    fun `normalize drops a pre-release suffix`() {
        assertEquals("1.17.0", AppVersion.normalize("1.17.0-dev"))
    }

    @Test
    fun `normalize drops a build suffix and surrounding whitespace`() {
        assertEquals("1.17.0", AppVersion.normalize("  1.17.0+abc1234\n"))
    }

    @Test
    fun `version code packs the three components`() {
        assertEquals(1_017_000, AppVersion.versionCode("1.17.0"))
        assertEquals(1_017_003, AppVersion.versionCode("1.17.3"))
        assertEquals(2_000_000, AppVersion.versionCode("2.0.0"))
    }

    @Test
    fun `version code ignores the suffix`() {
        assertEquals(AppVersion.versionCode("1.17.0"), AppVersion.versionCode("1.17.0-dev"))
    }

    @Test
    fun `version code grows monotonically with the release order`() {
        val ordered = listOf("0.9.9", "1.0.0", "1.0.1", "1.2.0", "1.17.0", "2.0.0")
        val codes = ordered.map(AppVersion::versionCode)
        assertEquals(codes.sorted(), codes)
    }

    @Test
    fun `a component wider than the scale factor is rejected`() {
        val failure = assertFailsWith<IllegalArgumentException> { AppVersion.versionCode("1.1000.0") }
        assertTrue(failure.message!!.contains("999"))
    }

    @Test
    fun `a malformed version is rejected`() {
        assertFailsWith<IllegalArgumentException> { AppVersion.versionCode("1.17") }
        assertFailsWith<IllegalArgumentException> { AppVersion.versionCode("v1.17.0") }
        assertFailsWith<IllegalArgumentException> { AppVersion.versionCode("") }
    }
}
