package ru.tinyops.turboist.core.model

import kotlin.test.Test
import kotlin.test.assertEquals

class LinkCleanupTest {
    @Test
    fun `plain text and clean links are left alone`() {
        assertEquals("buy milk", LinkCleanup.strip("buy milk"))
        assertEquals("https://example.com/a?id=5", LinkCleanup.strip("https://example.com/a?id=5"))
        assertEquals("", LinkCleanup.strip(""))
    }

    @Test
    fun `utm and click id parameters are removed`() {
        assertEquals(
            "https://example.com/a?id=5",
            LinkCleanup.strip("https://example.com/a?utm_source=x&utm_term=y&id=5"),
        )
        assertEquals(
            "https://example.com/",
            LinkCleanup.strip("https://example.com/?gclid=1&fbclid=2&yclid=3&msclkid=4"),
        )
    }

    @Test
    fun `fragment is kept and an emptied query is dropped`() {
        assertEquals("https://example.com/a#part", LinkCleanup.strip("https://example.com/a?utm_campaign=z#part"))
    }

    @Test
    fun `names match case-insensitively but only whole names`() {
        assertEquals("https://example.com/?a=1", LinkCleanup.strip("https://example.com/?UTM_Source=x&a=1"))
        assertEquals(
            "https://example.com/?utmx=1&my_utm_source=2",
            LinkCleanup.strip("https://example.com/?utmx=1&my_utm_source=2"),
        )
    }

    @Test
    fun `links inside text are cleaned and trailing punctuation is kept`() {
        assertEquals(
            "read https://example.com/a later",
            LinkCleanup.strip("read https://example.com/a?utm_source=x later"),
        )
        assertEquals("see https://example.com/a.", LinkCleanup.strip("see https://example.com/a?utm_source=x."))
        assertEquals(
            "[post](https://example.com/a?b=2)",
            LinkCleanup.strip("[post](https://example.com/a?utm_source=x&b=2)"),
        )
    }

    @Test
    fun `what stays is not re-encoded`() {
        assertEquals("https://b.io/?q=%D1%82", LinkCleanup.strip("https://b.io/?q=%D1%82&fbclid=2"))
    }

    @Test
    fun `typing one character is never rewritten`() {
        assertEquals(
            "https://example.com/?utm_",
            LinkCleanup.afterEdit("https://example.com/?utm", "https://example.com/?utm_"),
        )
    }

    @Test
    fun `a pasted link is cleaned`() {
        assertEquals(
            "see https://example.com/a",
            LinkCleanup.afterEdit("see ", "see https://example.com/a?utm_source=x"),
        )
    }
}
