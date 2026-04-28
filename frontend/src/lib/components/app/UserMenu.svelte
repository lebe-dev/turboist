<script lang="ts">
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu';
	import { Button } from '$lib/components/ui/button';
	import UserIcon from 'phosphor-svelte/lib/User';
	import { getAuthStore } from '$lib/auth/store.svelte';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { contextsStore } from '$lib/stores/contexts.svelte';
	import { projectsStore } from '$lib/stores/projects.svelte';
	import { labelsStore } from '$lib/stores/labels.svelte';
	import { configStore } from '$lib/stores/config.svelte';

	const auth = getAuthStore();

	function clearStores(): void {
		contextsStore.clear();
		projectsStore.clear();
		labelsStore.clear();
		configStore.clear();
	}

	async function onLogout(): Promise<void> {
		await auth.logout();
		clearStores();
		await goto(resolve('/login'));
	}

	async function onLogoutAll(): Promise<void> {
		await auth.logoutAll();
		clearStores();
		await goto(resolve('/login'));
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger>
		{#snippet child({ props })}
			<Button {...props} variant="ghost" size="sm" class="gap-2">
				<UserIcon class="size-4" />
				<span class="text-sm">{auth.user?.username ?? ''}</span>
			</Button>
		{/snippet}
	</DropdownMenu.Trigger>
	<DropdownMenu.Content align="end" class="w-48">
		<DropdownMenu.Label>{auth.user?.username ?? ''}</DropdownMenu.Label>
		<DropdownMenu.Separator />
		<DropdownMenu.Item onclick={onLogout}>Log out</DropdownMenu.Item>
		<DropdownMenu.Item onclick={onLogoutAll}>Log out everywhere</DropdownMenu.Item>
	</DropdownMenu.Content>
</DropdownMenu.Root>
