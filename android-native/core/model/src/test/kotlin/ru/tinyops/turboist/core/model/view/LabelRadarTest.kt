package ru.tinyops.turboist.core.model.view

import ru.tinyops.turboist.core.model.Label
import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertNull

/** The radar's layout, kept in step with the web client's. */
class LabelRadarTest {
    private var nextLocalId = 0L

    private fun usage(
        name: String,
        week: LabelPeriodUsage = LabelPeriodUsage(),
        month: LabelPeriodUsage = LabelPeriodUsage(),
    ): LabelUsage {
        nextLocalId += 1
        return LabelUsage(
            label = Label(localId = nextLocalId, name = name, createdAt = 0L, updatedAt = 0L),
            periods = mapOf(LabelUsagePeriod.WEEK to week, LabelUsagePeriod.MONTH to month),
        )
    }

    private fun LabelRadar?.names() = this?.axes?.map { it.usage.label.name }

    @Test
    fun `three labels with activity are needed to make a shape`() {
        val rows = listOf(usage("a", LabelPeriodUsage(3)), usage("b", LabelPeriodUsage(1)), usage("c"))
        assertNull(labelRadar(rows, LabelUsagePeriod.WEEK))
    }

    @Test
    fun `a label used only in the previous window still gets a spoke`() {
        val radar =
            labelRadar(
                listOf(
                    usage("a", LabelPeriodUsage(applied = 4)),
                    usage("b", LabelPeriodUsage(applied = 2)),
                    usage("gone", LabelPeriodUsage(previousApplied = 8)),
                ),
                LabelUsagePeriod.WEEK,
            )
        assertEquals(listOf("a", "b", "gone"), radar.names())
        assertEquals(8, radar?.scaleMax)
        val gone = radar?.axes?.single { it.usage.label.name == "gone" }
        assertEquals(0f, gone?.current)
        assertEquals(1f, gone?.previous)
    }

    @Test
    fun `spokes go by name so a label keeps its direction across periods`() {
        val rows =
            listOf(
                usage("zeta", LabelPeriodUsage(9), LabelPeriodUsage(1)),
                usage("alpha", LabelPeriodUsage(1), LabelPeriodUsage(9)),
                usage("mid", LabelPeriodUsage(5), LabelPeriodUsage(5)),
            )
        val week = labelRadar(rows, LabelUsagePeriod.WEEK)
        val month = labelRadar(rows, LabelUsagePeriod.MONTH)
        assertEquals(listOf("alpha", "mid", "zeta"), week.names())
        assertEquals(
            week?.axes?.map { it.usage.label.name to it.angle },
            month?.axes?.map { it.usage.label.name to it.angle },
        )
        assertEquals(0.0, week?.axes?.first()?.angle)
    }

    @Test
    fun `past the cap the busiest labels keep their spokes`() {
        val radar =
            labelRadar(
                listOf(
                    usage("a", LabelPeriodUsage(1)),
                    usage("b", LabelPeriodUsage(5)),
                    usage("c", LabelPeriodUsage(4)),
                    usage("d", LabelPeriodUsage(previousApplied = 3)),
                ),
                LabelUsagePeriod.WEEK,
                maxAxes = 3,
            )
        assertEquals(listOf("b", "c", "d"), radar.names())
        assertEquals(1, radar?.hidden)
    }
}
