/**
 * Tracks whether the connected Google Calendar account needs the user to
 * re-authorize (the stored OAuth grant expired or was revoked). Background
 * event fetches and the manual sync flip this flag so the app can surface a
 * clear, actionable notice instead of silently dropping calendar events.
 */
class CalendarReauthStore {
	needed = $state<boolean>(false);

	flag(): void {
		this.needed = true;
	}

	clear(): void {
		this.needed = false;
	}
}

export const calendarReauthStore = new CalendarReauthStore();
