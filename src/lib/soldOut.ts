import { browser } from '$app/environment';
import { writable } from 'svelte/store';
import { apiBaseUrl } from '$lib/staff/staffIdentity';

export const SOLD_OUT_POLL_MS = 60_000;

// falls back to nothing sold out
//   if there's no connection to api
export const soldOutIds = writable<Set<string>>(new Set());

let started = false;

// fail open errors keep the last known state
export async function startSoldOutSync() {
	if (!browser || started) return;
	started = true;

	const fetchSoldOut = async () => {
		try {
			const res = await fetch(`${apiBaseUrl()}/sold-out`);
			if (!res.ok) throw new Error(`status ${res.status}`);
			const body: unknown = await res.json();
			if (!Array.isArray(body)) throw new Error('unexpected payload');
			soldOutIds.set(new Set(body.filter((id): id is string => typeof id === 'string')));
		} catch (err) {
			console.warn('/sold-out api unavailable, using last known state', err);
		}
	};

	void fetchSoldOut();
	window.setInterval(fetchSoldOut, SOLD_OUT_POLL_MS);
}
