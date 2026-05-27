<script lang="ts">
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import CaretDownIcon from 'phosphor-svelte/lib/CaretDown';
	import CaretRightIcon from 'phosphor-svelte/lib/CaretRight';
	import { t } from '$lib/i18n';
	import { apiLogStore, type ApiLogEntry } from '$lib/stores/apiLog.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Switch } from '$lib/components/ui/switch';

	let expandedId = $state<number | null>(null);

	function toggle(id: number): void {
		expandedId = expandedId === id ? null : id;
	}

	function formatTime(d: Date): string {
		const h = String(d.getHours()).padStart(2, '0');
		const m = String(d.getMinutes()).padStart(2, '0');
		const s = String(d.getSeconds()).padStart(2, '0');
		const ms = String(d.getMilliseconds()).padStart(3, '0');
		return `${h}:${m}:${s}.${ms}`;
	}

	function statusColor(entry: ApiLogEntry): string {
		if (entry.error) return 'text-red-500';
		if (!entry.status) return 'text-muted-foreground';
		if (entry.status < 300) return 'text-green-600 dark:text-green-400';
		if (entry.status < 400) return 'text-yellow-600 dark:text-yellow-400';
		return 'text-red-500';
	}

	function methodColor(method: string): string {
		switch (method) {
			case 'GET': return 'bg-blue-500/15 text-blue-700 dark:text-blue-300';
			case 'POST': return 'bg-green-500/15 text-green-700 dark:text-green-300';
			case 'PATCH': return 'bg-yellow-500/15 text-yellow-700 dark:text-yellow-300';
			case 'PUT': return 'bg-orange-500/15 text-orange-700 dark:text-orange-300';
			case 'DELETE': return 'bg-red-500/15 text-red-700 dark:text-red-300';
			default: return 'bg-muted text-muted-foreground';
		}
	}

	function stripOrigin(url: string): string {
		try {
			const u = new URL(url, 'http://localhost');
			return u.pathname + u.search;
		} catch {
			return url;
		}
	}
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.logs.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.logs.description')}</p>
		</div>
		<div class="flex shrink-0 items-center gap-2">
			{#if apiLogStore.enabled && apiLogStore.entries.length > 0}
				<Button variant="outline" size="sm" onclick={() => apiLogStore.clear()}>
					<TrashIcon class="size-3.5" />
					{$t('settings.logs.clear')}
				</Button>
			{/if}
			<Switch
				checked={apiLogStore.enabled}
				onCheckedChange={(v) => apiLogStore.setEnabled(v)}
				aria-label={$t('settings.logs.enable')}
			/>
		</div>
	</div>

	{#if !apiLogStore.enabled}
		<p class="py-4 text-center text-sm text-muted-foreground">{$t('settings.logs.disabled')}</p>
	{:else if apiLogStore.entries.length === 0}
		<p class="py-4 text-center text-sm text-muted-foreground">{$t('settings.logs.empty')}</p>
	{:else}
		<div class="flex flex-col divide-y divide-border overflow-hidden rounded-md border border-border">
			{#each apiLogStore.entries as entry (entry.id)}
				{@const expanded = expandedId === entry.id}
				<div class="bg-background">
					<button
						type="button"
						onclick={() => toggle(entry.id)}
						class="flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition-colors hover:bg-muted/50"
					>
						{#if expanded}
							<CaretDownIcon class="size-3 shrink-0 text-muted-foreground" />
						{:else}
							<CaretRightIcon class="size-3 shrink-0 text-muted-foreground" />
						{/if}
						<span class="shrink-0 font-mono text-muted-foreground/70">{formatTime(entry.timestamp)}</span>
						<span class="shrink-0 rounded px-1.5 py-0.5 font-mono text-[10px] font-semibold {methodColor(entry.method)}">{entry.method}</span>
						<span class="min-w-0 truncate font-mono">{stripOrigin(entry.url)}</span>
						<span class="ml-auto flex shrink-0 items-center gap-2">
							{#if entry.error}
								<span class="truncate text-red-500" title={entry.error}>ERR</span>
							{:else}
								<span class={statusColor(entry)}>{entry.status}</span>
							{/if}
							<span class="text-muted-foreground/70">{entry.durationMs}ms</span>
						</span>
					</button>
					{#if expanded}
						<div class="flex flex-col gap-2 border-t border-border/50 bg-muted/20 px-3 py-2.5">
							{#if entry.error}
								<div class="flex flex-col gap-0.5">
									<span class="text-[10px] font-semibold uppercase tracking-wider text-red-500">{$t('settings.logs.error')}</span>
									<code class="break-all rounded-md border border-red-500/20 bg-red-500/5 px-2 py-1.5 font-mono text-[11px] leading-snug text-red-600 dark:text-red-400">{entry.error}</code>
								</div>
							{/if}
							{#if entry.requestBody}
								<div class="flex flex-col gap-0.5">
									<span class="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{$t('settings.logs.requestBody')}</span>
									<code class="block max-h-40 overflow-auto break-all rounded-md border border-border bg-muted/30 px-2 py-1.5 font-mono text-[11px] leading-snug">{entry.requestBody}</code>
								</div>
							{/if}
							{#if entry.responseBody}
								<div class="flex flex-col gap-0.5">
									<span class="text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">{$t('settings.logs.responseBody')}</span>
									<code class="block max-h-40 overflow-auto break-all rounded-md border border-border bg-muted/30 px-2 py-1.5 font-mono text-[11px] leading-snug">{entry.responseBody}</code>
								</div>
							{/if}
							{#if !entry.error && !entry.requestBody && !entry.responseBody}
								<span class="text-[11px] text-muted-foreground">{$t('settings.logs.noDetails')}</span>
							{/if}
						</div>
					{/if}
				</div>
			{/each}
		</div>
	{/if}
</section>
