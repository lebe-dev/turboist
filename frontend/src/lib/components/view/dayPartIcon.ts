import SunHorizonIcon from 'phosphor-svelte/lib/SunHorizon';
import SunIcon from 'phosphor-svelte/lib/Sun';
import MoonIcon from 'phosphor-svelte/lib/Moon';
import ClockIcon from 'phosphor-svelte/lib/Clock';
import type { Component } from 'svelte';
import type { DayPart } from '$lib/api/types';

const ICONS: Record<DayPart, Component> = {
	morning: SunHorizonIcon as unknown as Component,
	afternoon: SunIcon as unknown as Component,
	evening: MoonIcon as unknown as Component,
	none: ClockIcon as unknown as Component
};

export function iconFor(part: DayPart): Component {
	return ICONS[part];
}
