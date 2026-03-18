<script lang="ts">
	import { onMount } from 'svelte';
	import { logger, type LogEntry } from '$lib/stores/logger';
	import { t } from 'svelte-intl-precompile';
	import TrashIcon from '@lucide/svelte/icons/trash-2';

	let filter = $state<'all' | 'info' | 'warn' | 'error'>('all');
	let version = $state(0);

	onMount(() => {
		version = logger.version;
		return logger.subscribe(() => {
			version = logger.version;
		});
	});

	const filtered = $derived.by(() => {
		void version;
		const all = logger.entries;
		return filter === 'all' ? [...all] : all.filter((e) => e.level === filter);
	});

	function formatTime(ts: number): string {
		const d = new Date(ts);
		return (
			String(d.getHours()).padStart(2, '0') +
			':' +
			String(d.getMinutes()).padStart(2, '0') +
			':' +
			String(d.getSeconds()).padStart(2, '0') +
			'.' +
			String(d.getMilliseconds()).padStart(3, '0')
		);
	}

	const levelColor: Record<LogEntry['level'], string> = {
		info: 'bg-blue-500/15 text-blue-500',
		warn: 'bg-yellow-500/15 text-yellow-600 dark:text-yellow-400',
		error: 'bg-red-500/15 text-red-500'
	};

	const filters: { value: typeof filter; label: string }[] = [
		{ value: 'all', label: 'All' },
		{ value: 'info', label: 'Info' },
		{ value: 'warn', label: 'Warn' },
		{ value: 'error', label: 'Error' }
	];
</script>

<div class="flex flex-col gap-3">
	<div class="flex items-center justify-between">
		<div class="flex gap-1">
			{#each filters as f (f.value)}
				<button
					class="rounded-md px-2.5 py-1 text-[12px] font-medium transition-colors
						{filter === f.value
							? 'bg-accent text-foreground'
							: 'text-muted-foreground hover:bg-accent/50 hover:text-foreground'}"
					onclick={() => (filter = f.value)}
				>
					{f.label}
				</button>
			{/each}
		</div>
		<button
			class="flex items-center gap-1.5 rounded-md px-2.5 py-1 text-[12px] text-muted-foreground transition-colors hover:bg-accent hover:text-foreground"
			onclick={() => logger.clear()}
		>
			<TrashIcon class="h-3 w-3" />
			{$t('settings.logs.clear')}
		</button>
	</div>

	{#if filtered.length === 0}
		<div class="flex items-center justify-center py-12 text-sm text-muted-foreground">
			{$t('settings.logs.empty')}
		</div>
	{:else}
		<div class="max-h-[60vh] overflow-y-auto rounded-lg border border-border/50 bg-muted/30">
			{#each filtered as entry (entry.timestamp + entry.tag + entry.message)}
				<div class="flex items-baseline gap-2 border-b border-border/30 px-3 py-1.5 font-mono text-[11px] last:border-b-0">
					<span class="shrink-0 tabular-nums text-muted-foreground/60">{formatTime(entry.timestamp)}</span>
					<span class="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase {levelColor[entry.level]}">{entry.level}</span>
					<span class="shrink-0 font-semibold text-foreground/70">{entry.tag}</span>
					<span class="min-w-0 break-all text-foreground/90">{entry.message}</span>
				</div>
			{/each}
		</div>
	{/if}
</div>
