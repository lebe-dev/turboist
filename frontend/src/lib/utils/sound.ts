/**
 * Neutral status-change feedback, synthesized with WebAudio.
 *
 * The tones are generated instead of shipped as audio assets: two short sine
 * notes cost a few lines here, whereas files in `static/` would have to be
 * cached by the service worker and bundled into the iOS/Android builds.
 *
 * A single AudioContext is created lazily on the first play — always from a
 * click handler, which is what iOS requires for playback to be allowed at all.
 */

type AudioContextCtor = typeof AudioContext;

let ctx: AudioContext | null = null;

function audioContext(): AudioContext | null {
	if (ctx) return ctx;
	if (typeof window === 'undefined') return null;
	const Ctor: AudioContextCtor | undefined =
		window.AudioContext ??
		(window as unknown as { webkitAudioContext?: AudioContextCtor }).webkitAudioContext;
	if (!Ctor) return null;
	ctx = new Ctor();
	return ctx;
}

/** One sine note with a click-free attack and an exponential tail. */
function note(target: AudioContext, freq: number, startAt: number, duration: number, peak: number) {
	const osc = target.createOscillator();
	const gain = target.createGain();
	osc.type = 'sine';
	osc.frequency.setValueAtTime(freq, startAt);
	gain.gain.setValueAtTime(0.0001, startAt);
	gain.gain.linearRampToValueAtTime(peak, startAt + 0.012);
	gain.gain.exponentialRampToValueAtTime(0.0001, startAt + duration);
	osc.connect(gain);
	gain.connect(target.destination);
	osc.start(startAt);
	osc.stop(startAt + duration);
}

const PEAK = 0.09;
const NOTE = 0.13;
const GAP = 0.075;

/**
 * Two-note chime: ascending when a task is completed, descending when it is
 * reopened, so the direction of the change is audible without being a jingle.
 */
export function playStatusTone(completed: boolean): void {
	const target = audioContext();
	if (!target) return;
	// Autoplay policies park the context in `suspended` until a gesture resumes it.
	if (target.state === 'suspended') void target.resume();
	const [first, second] = completed ? [660, 880] : [660, 495];
	const now = target.currentTime;
	note(target, first, now, NOTE, PEAK);
	note(target, second, now + GAP, NOTE + 0.05, PEAK);
}
