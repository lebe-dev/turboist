import { patchState } from '$lib/api/client';

function createBannerStore() {
	let text = $state('');
	let dismissedText = $state('');

	return {
		get text() {
			return text;
		},
		get visible() {
			return text !== '' && text !== dismissedText;
		},
		init(bannerText: string, bannerDismissedText: string) {
			text = bannerText;
			dismissedText = bannerDismissedText;
		},
		setText(value: string) {
			text = value;
			patchState({ banner_text: value }).catch(console.error);
		},
		dismiss() {
			dismissedText = text;
			patchState({ banner_dismissed_text: text }).catch(console.error);
		}
	};
}

export const bannerStore = createBannerStore();
