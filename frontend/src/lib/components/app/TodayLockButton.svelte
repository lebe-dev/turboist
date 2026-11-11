<script lang="ts">
	import LockSimpleIcon from 'phosphor-svelte/lib/LockSimple';
	import LockSimpleOpenIcon from 'phosphor-svelte/lib/LockSimpleOpen';
	import { toast } from 'svelte-sonner';
	import * as Tooltip from '$lib/components/ui/tooltip';
	import { Button } from '$lib/components/ui/button';
	import ConfirmDestructiveDialog from '$lib/components/dialog/ConfirmDestructiveDialog.svelte';
	import { todayLockStore } from '$lib/stores/todayLock.svelte';
	import { nowStore } from '$lib/stores/now.svelte';
	import { describeError } from '$lib/utils/taskActions';
	import { t } from '$lib/i18n';

	// The Today focus lock control. Off, it offers to lock the day after a
	// confirmation; on, it only explains itself — the lock cannot be lifted
	// early, it runs out at midnight. nowStore ticks over at midnight, which is
	// what turns the control back off without a reload.

	let confirmOpen = $state(false);

	const active = $derived(todayLockStore.isActive(nowStore.now));
	const tooltip = $derived(
		active ? $t('page.today.lock.activeTooltip') : $t('page.today.lock.tooltip')
	);

	function handleClick(): void {
		if (active) {
			// A touch screen has no hover, so the tooltip never shows there; the
			// tap says the same thing instead.
			toast.info($t('page.today.lock.activeTooltip'));
			return;
		}
		confirmOpen = true;
	}

	async function lock(): Promise<void> {
		try {
			await todayLockStore.lock();
			toast.success($t('page.today.lock.locked'));
		} catch (err) {
			toast.error(describeError(err, $t('page.today.lock.failed')));
		}
	}
</script>

<Tooltip.Provider delayDuration={200}>
	<Tooltip.Root>
		<Tooltip.Trigger>
			{#snippet child({ props })}
				<Button
					{...props}
					variant="ghost"
					size="icon-sm"
					class={active ? 'shrink-0 text-primary' : 'shrink-0 text-muted-foreground'}
					onclick={handleClick}
					aria-label={active ? $t('page.today.lock.activeTooltip') : $t('page.today.lock.button')}
					aria-pressed={active}
					data-testid="today-lock"
				>
					{#if active}
						<LockSimpleIcon class="size-4" weight="fill" />
					{:else}
						<LockSimpleOpenIcon class="size-4" />
					{/if}
				</Button>
			{/snippet}
		</Tooltip.Trigger>
		<Tooltip.Content side="bottom" class="max-w-64 text-center">{tooltip}</Tooltip.Content>
	</Tooltip.Root>
</Tooltip.Provider>

<ConfirmDestructiveDialog
	bind:open={confirmOpen}
	variant="default"
	title={$t('page.today.lock.confirmTitle')}
	description={$t('page.today.lock.confirmDescription')}
	confirmLabel={$t('page.today.lock.confirm')}
	busyLabel={$t('page.today.lock.locking')}
	onConfirm={lock}
/>
