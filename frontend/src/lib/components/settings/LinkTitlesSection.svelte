<script lang="ts">
	import TrashIcon from 'phosphor-svelte/lib/Trash';
	import PlusIcon from 'phosphor-svelte/lib/Plus';
	import { Switch } from '$lib/components/ui/switch';
	import { toast } from 'svelte-sonner';
	import { appSettingsStore } from '$lib/stores/appSettings.svelte';
	import { t, locale } from '$lib/i18n';
	import type { LinkTitleRule } from '$lib/api/types';
	import {
		defaultLinkTitleTemplate,
		isValidLinkTitleTemplate,
		LINK_TITLE_TOKENS
	} from '$lib/utils/linkTitles';

	const tokens = { titleToken: LINK_TITLE_TOKENS.title, domainToken: LINK_TITLE_TOKENS.domain };

	let template = $state(appSettingsStore.linkTitles.template);
	let rules = $state<LinkTitleRule[]>(appSettingsStore.linkTitles.rules.map((r) => ({ ...r })));
	let busy = $state(false);
	let toggleBusy = $state(false);

	const enabled = $derived(appSettingsStore.linkTitles.enabled);
	const defaultTemplate = $derived(defaultLinkTitleTemplate($locale));

	const dirty = $derived.by(() => {
		const stored = appSettingsStore.linkTitles;
		if (template.trim() !== stored.template) return true;
		if (rules.length !== stored.rules.length) return true;
		return rules.some(
			(r, i) => r.domain !== stored.rules[i].domain || r.template !== stored.rules[i].template
		);
	});

	let pendingRuleFocus = $state(false);

	function addRule(): void {
		rules = [...rules, { domain: '', template: '' }];
		pendingRuleFocus = true;
	}

	function focusNewDomain(el: HTMLInputElement): void {
		if (pendingRuleFocus && el.offsetParent) {
			pendingRuleFocus = false;
			el.focus();
		}
	}

	function removeRule(idx: number): void {
		rules = rules.filter((_, i) => i !== idx);
	}

	async function toggle(next: boolean): Promise<void> {
		toggleBusy = true;
		try {
			await appSettingsStore.setLinkTitles({ enabled: next });
			toast.success(
				$t(next ? 'settings.linkTitles.toastToggledOn' : 'settings.linkTitles.toastToggledOff')
			);
		} catch (err) {
			toast.error(err instanceof Error ? err.message : $t('settings.linkTitles.toastFailed'));
		} finally {
			toggleBusy = false;
		}
	}

	async function save(): Promise<void> {
		const cleanedTemplate = template.trim();
		const cleanedRules = rules.map((r) => ({
			domain: r.domain.trim(),
			template: r.template.trim()
		}));
		if (
			(cleanedTemplate !== '' && !isValidLinkTitleTemplate(cleanedTemplate)) ||
			cleanedRules.some((r) => r.domain === '' || !isValidLinkTitleTemplate(r.template))
		) {
			toast.error($t('settings.linkTitles.toastInvalid', { values: tokens }));
			return;
		}
		busy = true;
		try {
			await appSettingsStore.setLinkTitles({ template: cleanedTemplate, rules: cleanedRules });
			// The server normalises domains (drops scheme, www., duplicates).
			template = appSettingsStore.linkTitles.template;
			rules = appSettingsStore.linkTitles.rules.map((r) => ({ ...r }));
			toast.success($t('settings.linkTitles.toastSaved'));
		} catch (err) {
			toast.error(err instanceof Error ? err.message : $t('settings.linkTitles.toastFailed'));
		} finally {
			busy = false;
		}
	}

	const inputClass =
		'min-w-0 rounded-md border border-input bg-background px-2 py-1.5 text-sm shadow-sm focus-visible:outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50';
</script>

<section class="flex flex-col gap-3 rounded-lg border border-border bg-card p-5 shadow-sm">
	<div class="flex items-start justify-between gap-3">
		<div class="flex flex-col gap-0.5">
			<h2 class="text-sm font-semibold">{$t('settings.linkTitles.heading')}</h2>
			<p class="text-xs text-muted-foreground">{$t('settings.linkTitles.description')}</p>
		</div>
		<Switch
			checked={enabled}
			disabled={toggleBusy}
			onCheckedChange={toggle}
			aria-label={$t('settings.linkTitles.enabled')}
		/>
	</div>

	{#if enabled}
		<label class="flex flex-col gap-1">
			<span class="text-xs font-medium">{$t('settings.linkTitles.template')}</span>
			<input type="text" bind:value={template} placeholder={defaultTemplate} class={inputClass} />
			<span class="text-[11px] text-muted-foreground">
				{$t('settings.linkTitles.templateHint', {
					values: { ...tokens, fallback: defaultTemplate }
				})}
			</span>
		</label>

		<div class="flex flex-col gap-0.5 pt-1">
			<h3 class="text-xs font-semibold">{$t('settings.linkTitles.rulesHeading')}</h3>
			<p class="text-[11px] text-muted-foreground">{$t('settings.linkTitles.rulesDescription')}</p>
		</div>

		{#if rules.length === 0}
			<p class="text-sm text-muted-foreground">{$t('settings.linkTitles.empty')}</p>
		{:else}
			<div class="flex flex-col gap-2">
				<div
					class="hidden grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] items-center gap-2 px-1 text-[11px] font-medium text-muted-foreground sm:grid"
				>
					<span>{$t('settings.linkTitles.domain')}</span>
					<span>{$t('settings.linkTitles.ruleTemplate')}</span>
					<span class="sr-only">{$t('settings.linkTitles.remove')}</span>
				</div>
				{#each rules as rule, idx (idx)}
					<div
						class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-md border border-border p-3 sm:grid-cols-[minmax(0,1fr)_minmax(0,1.4fr)_auto] sm:border-0 sm:p-0"
					>
						<input
							type="text"
							inputmode="url"
							autocapitalize="off"
							spellcheck="false"
							bind:value={rule.domain}
							{@attach (el) => {
								if (idx === rules.length - 1) focusNewDomain(el);
							}}
							placeholder={$t('settings.linkTitles.domainPlaceholder')}
							aria-label={$t('settings.linkTitles.domain')}
							class={inputClass}
						/>
						<input
							type="text"
							bind:value={rule.template}
							placeholder={$t('settings.linkTitles.ruleTemplatePlaceholder', { values: tokens })}
							aria-label={$t('settings.linkTitles.ruleTemplate')}
							class="{inputClass} order-last col-span-2 sm:order-none sm:col-span-1"
						/>
						<button
							type="button"
							onclick={() => removeRule(idx)}
							aria-label={$t('settings.linkTitles.remove')}
							class="justify-self-end rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-muted hover:text-destructive focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
						>
							<TrashIcon class="size-4" />
						</button>
					</div>
				{/each}
			</div>
		{/if}

		<div class="flex items-center justify-between gap-2 pt-1">
			<button
				type="button"
				onclick={addRule}
				class="inline-flex items-center gap-1 rounded-md border border-input bg-background px-3 py-1.5 text-xs font-medium shadow-sm transition-colors hover:bg-muted focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none"
			>
				<PlusIcon class="size-3.5" />
				{$t('settings.linkTitles.add')}
			</button>
			<button
				type="button"
				onclick={save}
				disabled={!dirty || busy}
				class="inline-flex items-center gap-1 rounded-md bg-foreground px-3 py-1.5 text-xs font-medium text-background shadow-sm transition-colors hover:bg-foreground/90 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50"
			>
				{$t('settings.linkTitles.save')}
			</button>
		</div>
	{/if}
</section>
