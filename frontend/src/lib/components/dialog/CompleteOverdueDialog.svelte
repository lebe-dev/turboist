<script lang="ts">
	import * as AlertDialog from '$lib/components/ui/alert-dialog';
	import { Button } from '$lib/components/ui/button';
	import { t } from '$lib/i18n';
	import type { Task } from '$lib/api/types';
	import { configStore } from '$lib/stores/config.svelte';
	import { dayKeyInTz, dayStartUtcInTz, formatDay, parseIso, toIsoUtc } from '$lib/utils/format';
	import { nowStore } from '$lib/stores/now.svelte';

	let {
		open = $bindable(false),
		task,
		onConfirm
	}: {
		open?: boolean;
		task: Task | null;
		onConfirm: (completedAt: string) => void | Promise<void>;
	} = $props();

	const tz = $derived(configStore.value?.timezone ?? null);

	type Choice = { key: string; label: string; iso: string };

	const choices = $derived.by<Choice[]>(() => {
		const out: Choice[] = [];
		const todayKey = nowStore.todayKey;
		const yesterdayKey = (() => {
			const d = dayStartUtcInTz(todayKey, tz);
			d.setUTCDate(d.getUTCDate() - 1);
			return dayKeyInTz(d, tz);
		})();

		const completionAt = (key: string): string => {
			const start = dayStartUtcInTz(key, tz);
			start.setUTCHours(start.getUTCHours() + 23, 59, 0, 0);
			return toIsoUtc(start);
		};

		out.push({ key: todayKey, label: $t('task.completeOverdue.today'), iso: toIsoUtc(new Date()) });
		out.push({
			key: yesterdayKey,
			label: $t('task.completeOverdue.yesterday'),
			iso: completionAt(yesterdayKey)
		});

		const due = parseIso(task?.dueAt);
		if (due) {
			const dueKey = dayKeyInTz(due, tz);
			if (dueKey !== todayKey && dueKey !== yesterdayKey) {
				out.push({
					key: dueKey,
					label: $t('task.completeOverdue.onDueDate', {
						values: { date: formatDay(due, false, tz) }
					}),
					iso: completionAt(dueKey)
				});
			}
		}
		return out;
	});

	let busy = $state(false);

	async function pick(iso: string) {
		if (busy) return;
		busy = true;
		try {
			await onConfirm(iso);
			open = false;
		} finally {
			busy = false;
		}
	}
</script>

<AlertDialog.Root bind:open>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>{$t('task.completeOverdue.title')}</AlertDialog.Title>
			<AlertDialog.Description>
				{$t('task.completeOverdue.description')}
			</AlertDialog.Description>
		</AlertDialog.Header>
		<div class="flex flex-col gap-2 py-2">
			{#each choices as c (c.key)}
				<Button variant="outline" disabled={busy} onclick={() => pick(c.iso)}>
					{c.label}
				</Button>
			{/each}
		</div>
		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={busy}>{$t('common.cancel')}</AlertDialog.Cancel>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
