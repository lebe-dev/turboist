<script lang="ts">
	import RepeatIcon from 'phosphor-svelte/lib/Repeat';
	import XIcon from 'phosphor-svelte/lib/X';

	type RecurrenceMode = 'none' | 'daily' | 'interval' | 'weekly' | 'monthly';

	const WEEKDAY_ORDER = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'] as const;
	type Weekday = (typeof WEEKDAY_ORDER)[number];

	interface ParsedRule {
		mode: RecurrenceMode;
		interval?: number;
		days?: Weekday[];
		day?: number;
	}

	const WEEKDAY_LABEL: Record<Weekday, string> = {
		MO: 'Mo',
		TU: 'Tu',
		WE: 'We',
		TH: 'Th',
		FR: 'Fr',
		SA: 'Sa',
		SU: 'Su'
	};

	let { value = $bindable<string | null>(null) }: { value?: string | null } = $props();

	let open = $state(false);
	let intervalDays = $state(2);
	let weekdays = $state<Weekday[]>(['MO', 'WE', 'FR']);
	let monthDay = $state(1);

	function parseRRule(v: string | null): ParsedRule {
		if (!v) return { mode: 'none' };
		const parts: Record<string, string> = {};
		for (const segment of v.split(';')) {
			const eq = segment.indexOf('=');
			if (eq !== -1) parts[segment.slice(0, eq)] = segment.slice(eq + 1);
		}
		const freq = parts['FREQ'];
		if (freq === 'DAILY') {
			const interval = Number(parts['INTERVAL'] ?? 1);
			return interval > 1 ? { mode: 'interval', interval } : { mode: 'daily' };
		}
		if (freq === 'WEEKLY') {
			const days = (parts['BYDAY'] ?? '')
				.split(',')
				.filter((d): d is Weekday => (WEEKDAY_ORDER as readonly string[]).includes(d));
			return { mode: 'weekly', days };
		}
		if (freq === 'MONTHLY') {
			return { mode: 'monthly', day: Number(parts['BYMONTHDAY'] ?? 1) };
		}
		return { mode: 'none' };
	}

	function getSummary(p: ParsedRule): string {
		switch (p.mode) {
			case 'daily':
				return 'Every day';
			case 'interval':
				return `Every ${p.interval} days`;
			case 'weekly': {
				const days = p.days ?? [];
				if (!days.length) return 'Weekly';
				return days.map((d) => WEEKDAY_LABEL[d]).join(', ');
			}
			case 'monthly':
				return `Day ${p.day} monthly`;
			default:
				return 'Repeat';
		}
	}

	const parsed = $derived(parseRRule(value));
	const currentMode = $derived(parsed.mode);
	const summary = $derived(getSummary(parsed));
	const hasRepeat = $derived(currentMode !== 'none');

	// Sync sub-control defaults from parsed value when it changes externally
	$effect(() => {
		if (parsed.mode === 'interval' && parsed.interval != null) intervalDays = parsed.interval;
		if (parsed.mode === 'weekly' && parsed.days?.length) weekdays = [...parsed.days] as Weekday[];
		if (parsed.mode === 'monthly' && parsed.day != null) monthDay = parsed.day;
	});

	function selectNone(): void {
		value = null;
		open = false;
	}

	function selectDaily(): void {
		value = 'FREQ=DAILY';
	}

	function applyInterval(): void {
		value = `FREQ=DAILY;INTERVAL=${Math.max(2, intervalDays)}`;
	}

	function applyWeekly(): void {
		const days = weekdays.length ? weekdays : (['MO'] as Weekday[]);
		if (!weekdays.length) weekdays = ['MO'];
		value = `FREQ=WEEKLY;BYDAY=${days.join(',')}`;
	}

	function toggleWeekday(day: Weekday): void {
		const next = weekdays.includes(day)
			? weekdays.filter((d) => d !== day)
			: ([...weekdays, day].sort(
					(a, b) => WEEKDAY_ORDER.indexOf(a) - WEEKDAY_ORDER.indexOf(b)
				) as Weekday[]);
		weekdays = next.length ? next : ['MO'];
		value = `FREQ=WEEKLY;BYDAY=${weekdays.join(',')}`;
	}

	function applyMonthly(): void {
		value = `FREQ=MONTHLY;BYMONTHDAY=${Math.min(31, Math.max(1, monthDay))}`;
	}

	function clearRepeat(e: MouseEvent): void {
		e.stopPropagation();
		value = null;
		open = false;
	}

	function dot(mode: RecurrenceMode): string {
		return `size-3.5 flex-shrink-0 rounded-full border-2 transition-colors ${currentMode === mode ? 'border-foreground/70 bg-foreground/20' : 'border-muted-foreground/40'}`;
	}
</script>

<div class="relative inline-flex">
	<div
		class="inline-flex h-8 items-center rounded-md border border-border bg-background text-xs font-medium transition-colors"
	>
		<button
			type="button"
			onclick={() => (open = !open)}
			aria-expanded={open}
			class={`inline-flex h-full items-center gap-1.5 px-2.5 transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50 ${hasRepeat ? 'rounded-l-md' : 'rounded-md'}`}
		>
			<RepeatIcon class="size-3.5" />
			<span>{hasRepeat ? summary : 'Repeat'}</span>
		</button>
		{#if hasRepeat}
			<button
				type="button"
				aria-label="Clear repeat"
				onclick={clearRepeat}
				class="inline-flex h-full items-center rounded-r-md border-l border-border px-1.5 transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50"
			>
				<XIcon class="size-3" />
			</button>
		{/if}
	</div>

	{#if open}
		<div
			class="fixed inset-0 z-10"
			role="presentation"
			onclick={() => (open = false)}
		></div>

		<div
			class="absolute left-0 top-9 z-20 w-64 rounded-lg border border-border bg-popover p-2 shadow-lg"
		>
			<!-- None -->
			<button
				type="button"
				onclick={selectNone}
				class={`flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-xs transition-colors hover:bg-accent ${currentMode === 'none' ? 'bg-accent' : ''}`}
			>
				<span class={dot('none')}></span>
				<span>No repeat</span>
			</button>

			<!-- Every day -->
			<button
				type="button"
				onclick={selectDaily}
				class={`flex w-full items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-xs transition-colors hover:bg-accent ${currentMode === 'daily' ? 'bg-accent' : ''}`}
			>
				<span class={dot('daily')}></span>
				<span>Every day</span>
			</button>

			<!-- Every N days -->
			<div
				role="button"
				tabindex="0"
				onclick={applyInterval}
				onkeydown={(e) => e.key === 'Enter' && applyInterval()}
				class={`flex w-full cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-xs transition-colors hover:bg-accent ${currentMode === 'interval' ? 'bg-accent' : ''}`}
			>
				<span class={dot('interval')}></span>
				<span class="flex-shrink-0">Every</span>
				<input
					type="number"
					min="2"
					max="365"
					value={intervalDays}
					oninput={(e) => {
						const n = parseInt((e.target as HTMLInputElement).value, 10);
						if (!isNaN(n) && n >= 2) {
							intervalDays = n;
							value = `FREQ=DAILY;INTERVAL=${n}`;
						}
					}}
					onclick={(e) => {
						e.stopPropagation();
						applyInterval();
					}}
					class="w-12 rounded border border-border bg-background px-1.5 py-0.5 text-center text-xs focus:outline-none focus:ring-[2px] focus:ring-ring/50"
				/>
				<span class="flex-shrink-0">days</span>
			</div>

			<!-- Weekly -->
			<div
				class={`flex w-full cursor-pointer flex-col gap-2 rounded-md px-2.5 py-2 text-xs transition-colors hover:bg-accent ${currentMode === 'weekly' ? 'bg-accent' : ''}`}
				role="button"
				tabindex="0"
				onclick={applyWeekly}
				onkeydown={(e) => e.key === 'Enter' && applyWeekly()}
			>
				<div class="flex items-center gap-2.5">
					<span class={dot('weekly')}></span>
					<span>On days of week</span>
				</div>
				<div class="ml-6 flex gap-0.5">
					{#each WEEKDAY_ORDER as day (day)}
						<button
							type="button"
							onclick={(e) => {
								e.stopPropagation();
								toggleWeekday(day);
							}}
							class={`flex h-6 w-[26px] items-center justify-center rounded text-[10px] font-medium transition-colors focus-visible:outline-none focus-visible:ring-[2px] focus-visible:ring-ring/50 ${weekdays.includes(day) ? 'bg-foreground/20 text-foreground' : 'bg-muted text-muted-foreground hover:bg-accent'}`}
						>
							{WEEKDAY_LABEL[day]}
						</button>
					{/each}
				</div>
			</div>

			<!-- Monthly -->
			<div
				class={`flex w-full cursor-pointer items-center gap-2 rounded-md px-2.5 py-2 text-xs transition-colors hover:bg-accent ${currentMode === 'monthly' ? 'bg-accent' : ''}`}
				role="button"
				tabindex="0"
				onclick={applyMonthly}
				onkeydown={(e) => e.key === 'Enter' && applyMonthly()}
			>
				<span class={dot('monthly')}></span>
				<span class="flex-shrink-0">Monthly, day</span>
				<input
					type="number"
					min="1"
					max="31"
					value={monthDay}
					oninput={(e) => {
						const n = parseInt((e.target as HTMLInputElement).value, 10);
						if (!isNaN(n) && n >= 1 && n <= 31) {
							monthDay = n;
							value = `FREQ=MONTHLY;BYMONTHDAY=${n}`;
						}
					}}
					onclick={(e) => {
						e.stopPropagation();
						applyMonthly();
					}}
					class="w-12 rounded border border-border bg-background px-1.5 py-0.5 text-center text-xs focus:outline-none focus:ring-[2px] focus:ring-ring/50"
				/>
			</div>
		</div>
	{/if}
</div>
