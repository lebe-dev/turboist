/**
 * Whether a task about to be written would be refused by the Today focus lock.
 *
 * Mirrors the server rule (service.TodayLockService) as far as the client can
 * see it: while the lock runs, a task dated today or earlier may only land in a
 * project that already has open work on Today — never in the inbox. The set of
 * today's projects is only known while the Today page publishes it, so an
 * unknown set (`null`) answers for the inbox alone and lets the server decide
 * the rest.
 *
 * Day keys are `YYYY-MM-DD` in the server timezone, which compare as strings.
 */
export function wouldTodayLockRefuse(input: {
	lockActive: boolean;
	dueKey: string;
	todayKey: string;
	projectId: number | null;
	todayProjectIds: number[] | null;
}): boolean {
	if (!input.lockActive || !input.dueKey || input.dueKey > input.todayKey) return false;
	if (input.projectId === null) return true;
	if (input.todayProjectIds === null) return false;
	return !input.todayProjectIds.includes(input.projectId);
}
