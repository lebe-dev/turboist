import { logger } from '$lib/stores/logger';
import { patchState } from '$lib/api/client';

function createDayPartNotesStore() {
	let notes = $state<Record<string, string>>({});
	let maxLength = $state(200);

	function init(initial: Record<string, string>, max: number): void {
		notes = initial ?? {};
		maxLength = max;
	}

	function setNote(dayPartLabel: string, text: string): void {
		const trimmed = text.slice(0, maxLength);
		notes = { ...notes, [dayPartLabel]: trimmed };
		patchState({ day_part_notes: notes }).catch((e) => {
			logger.error('day-part-notes', `save failed: ${e}`);
		});
	}

	function clearNote(dayPartLabel: string): void {
		const { [dayPartLabel]: _, ...rest } = notes;
		notes = rest;
		patchState({ day_part_notes: notes }).catch((e) => {
			logger.error('day-part-notes', `clear failed: ${e}`);
		});
	}

	return {
		get notes() {
			return notes;
		},
		get maxLength() {
			return maxLength;
		},
		init,
		setNote,
		clearNote
	};
}

export const dayPartNotesStore = createDayPartNotesStore();
