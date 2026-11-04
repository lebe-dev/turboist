// Mirrors internal/service/linktitle/template.go — keep the two in step.

export const LINK_TITLE_TOKENS = { title: '{title}', domain: '{domain}' } as const;

const DEFAULT_TEMPLATES: Record<string, string> = {
	ru: 'Статья «{title}»',
	en: 'Article “{title}”'
};

/** The built-in template the server uses when none is configured. */
export function defaultLinkTitleTemplate(locale: string | null | undefined): string {
	const key = (locale ?? '').toLowerCase().split('-')[0];
	return DEFAULT_TEMPLATES[key] ?? DEFAULT_TEMPLATES.en;
}

const MAX_TEMPLATE_LENGTH = 200;

export function isValidLinkTitleTemplate(template: string): boolean {
	const t = template.trim();
	return t.includes(LINK_TITLE_TOKENS.title) && [...t].length <= MAX_TEMPLATE_LENGTH;
}
