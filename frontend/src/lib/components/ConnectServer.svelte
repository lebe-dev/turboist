<script lang="ts">
	import { t } from '$lib/i18n';
	import { normalizeServerUrl } from '$lib/native/serverUrl';
	import { Button } from '$lib/components/ui/button';

	let { onconnect }: { onconnect: (url: string) => void } = $props();

	let value = $state('');
	let error = $state('');
	let checking = $state(false);

	async function submit(e: Event) {
		e.preventDefault();
		error = '';
		const url = normalizeServerUrl(value);
		if (!/^https?:\/\//.test(url)) {
			error = $t('connect.invalidUrl');
			return;
		}
		checking = true;
		try {
			// CapacitorHttp routes this natively (no CORS). /api/config is public
			// and always answers 200, so it doubles as a reachability probe.
			const res = await fetch(`${url}/api/config`, { headers: { accept: 'application/json' } });
			if (!res.ok) throw new Error('unreachable');
			await res.json();
			onconnect(url);
		} catch {
			error = $t('connect.unreachable');
		} finally {
			checking = false;
		}
	}
</script>

<div class="flex h-screen items-center justify-center p-6">
	<form onsubmit={submit} class="w-full max-w-sm space-y-4">
		<h1 class="text-lg font-semibold">{$t('connect.title')}</h1>
		<p class="text-sm text-muted-foreground">{$t('connect.subtitle')}</p>
		<input
			bind:value
			type="url"
			inputmode="url"
			autocapitalize="none"
			autocorrect="off"
			placeholder="https://turboist.example.com"
			class="w-full rounded-md border bg-background px-3 py-2 text-sm"
		/>
		{#if error}<p class="text-sm text-destructive">{error}</p>{/if}
		<Button type="submit" class="w-full" disabled={checking}>
			{checking ? $t('connect.checking') : $t('connect.connect')}
		</Button>
	</form>
</div>
