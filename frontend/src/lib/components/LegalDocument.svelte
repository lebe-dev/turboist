<script lang="ts">
	import ArrowLeftIcon from 'phosphor-svelte/lib/ArrowLeft';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { page } from '$app/state';
	import MarkdownRich from './MarkdownRich.svelte';
	import { t } from '$lib/i18n';

	let { title, content }: { title: string; content: string } = $props();

	const rendered = $derived(content.replaceAll('{{ APP_URL }}', page.url.origin));

	function goBack(): void {
		void goto(resolve('/'));
	}
</script>

<div class="mx-auto flex max-w-3xl flex-col gap-6 px-4 py-8">
	<button
		type="button"
		class="flex items-center gap-1.5 self-start text-sm text-muted-foreground hover:text-foreground"
		onclick={goBack}
	>
		<ArrowLeftIcon class="h-4 w-4" />
		{$t('legal.back')}
	</button>

	<h1 class="text-2xl font-bold">{title}</h1>

	<div class="text-sm leading-relaxed">
		<MarkdownRich text={rendered} />
	</div>
</div>
