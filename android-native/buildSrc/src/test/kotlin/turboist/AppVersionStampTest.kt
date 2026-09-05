package turboist

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class AppVersionStampTest {
    @Test
    fun `an unstamped build shows the bare release version`() {
        assertEquals("1.17.0", AppVersion.buildVersionName("1.17.0", null))
        assertEquals("1.17.0", AppVersion.buildVersionName("1.17.0\n", ""))
        assertEquals("1.17.0", AppVersion.buildVersionName("1.17.0", "   "))
    }

    @Test
    fun `a stamped build carries the commit it was built from`() {
        assertEquals("1.17.0+a1b2c3d", AppVersion.buildVersionName("1.17.0", "a1b2c3d"))
        assertEquals("1.17.0+a1b2c3d", AppVersion.buildVersionName("1.17.0-dev", " a1b2c3d "))
    }

    @Test
    fun `the stamp never changes the release ordering`() {
        assertEquals(
            AppVersion.versionCode("1.17.0"),
            AppVersion.versionCode(AppVersion.buildVersionName("1.17.0", "a1b2c3d")),
        )
    }

    @Test
    fun `a stamp that is not a plain identifier is rejected`() {
        // The value ends up in a manifest attribute and in an artifact file
        // name, so anything that would need quoting in either is refused here
        // rather than producing a build that cannot be uploaded.
        assertFailsWith<IllegalArgumentException> { AppVersion.buildVersionName("1.17.0", "a1b2c3d dirty") }
        assertFailsWith<IllegalArgumentException> { AppVersion.buildVersionName("1.17.0", "feature/x") }
    }
}
