<script lang="ts">
	import RepeatIcon from '@lucide/svelte/icons/repeat';
	import XIcon from '@lucide/svelte/icons/x';
	import { t, locale } from 'svelte-intl-precompile';

	let {
		onSelect,
		onRemove,
		isRecurring = false,
	}: {
		onSelect: (dueString: string) => void;
		onRemove?: () => void;
		isRecurring?: boolean;
	} = $props();

	let showCustom = $state(false);
	let customValue = $state('');

	const now = new Date();
	const dayOfMonth = now.getDate();

	const dayNameEn = now.toLocaleDateString('en-US', { weekday: 'long' });
	const dayNameRu = now.toLocaleDateString('ru-RU', { weekday: 'long' });
	const dayName = $derived($locale === 'ru' ? dayNameRu : dayNameEn);

	const monthDayEn = now.toLocaleDateString('en-US', { month: 'long', day: 'numeric' });
	const monthDayRu = now.toLocaleDateString('ru-RU', { month: 'long', day: 'numeric' });

	function ordinal(n: number): string {
		if (n >= 11 && n <= 13) return n + 'th';
		switch (n % 10) {
			case 1: return n + 'st';
			case 2: return n + 'nd';
			case 3: return n + 'rd';
			default: return n + 'th';
		}
	}

	// Todoist API always uses English due strings
	const presets = [
		{ label: () => $t('task.recurrence.everyDay'), value: 'every day' },
		{ label: () => $t('task.recurrence.everyWeek', { values: { day: dayName } }), value: `every week on ${dayNameEn}` },
		{ label: () => $t('task.recurrence.everyWeekday'), value: 'every weekday' },
		{ label: () => $t('task.recurrence.everyMonth', { values: { date: String(dayOfMonth) } }), value: `every month on the ${ordinal(dayOfMonth)}` },
		{ label: () => $t('task.recurrence.everyYear', { values: { date: $locale === 'ru' ? monthDayRu : monthDayEn } }), value: `every year on ${monthDayEn}` },
	];

	function handlePreset(value: string) {
		showCustom = false;
		customValue = '';
		onSelect(value);
	}

	function handleCustomSubmit() {
		const val = customValue.trim();
		if (!val) return;
		onSelect(val);
		customValue = '';
		showCustom = false;
	}

	function handleCustomKeydown(e: KeyboardEvent) {
		if (e.key === 'Enter') {
			e.preventDefault();
			handleCustomSubmit();
		}
		if (e.key === 'Escape') {
			showCustom = false;
		}
	}
</script>

<div class="px-2 py-1.5">
	<div class="flex items-center gap-1.5">
		<RepeatIcon class="h-3.5 w-3.5 text-muted-foreground" />
		<p class="text-xs font-semibold text-muted-foreground">{$t('task.recurrence')}</p>
	</div>
	<div class="mt-1.5 flex flex-col gap-0.5">
		{#each presets as preset}
			<button
				class="rounded-md px-2 py-1.5 text-left text-[13px] text-foreground/90 transition-colors hover:bg-accent"
				onclick={() => handlePreset(preset.value)}
			>
				{preset.label()}
			</button>
		{/each}

		{#if showCustom}
			<div class="mt-1 flex items-center gap-1">
				<input
					type="text"
					bind:value={customValue}
					onkeydown={handleCustomKeydown}
					placeholder={$t('task.recurrence.customPlaceholder')}
					class="h-7 flex-1 rounded-md border border-border bg-background px-2 text-[13px] text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-ring"
					autofocus
				/>
				<button
					class="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-primary transition-colors hover:bg-accent"
					onclick={handleCustomSubmit}
					aria-label="Submit"
				>
					<RepeatIcon class="h-3.5 w-3.5" />
				</button>
			</div>
		{:else}
			<button
				class="rounded-md px-2 py-1.5 text-left text-[13px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
				onclick={() => { showCustom = true; }}
			>
				{$t('task.recurrence.custom')}
			</button>
		{/if}

		{#if isRecurring && onRemove}
			<button
				class="mt-0.5 flex items-center gap-1.5 rounded-md px-2 py-1.5 text-left text-[13px] text-destructive transition-colors hover:bg-destructive/10"
				onclick={onRemove}
			>
				<XIcon class="h-3.5 w-3.5" />
				{$t('task.recurrence.remove')}
			</button>
		{/if}
	</div>
</div>
