import { browser } from '$app/environment';

const STORAGE_KEY = 'artist-colony-checkout-draft-v1';

export type CheckoutDraft = {
	name: string;
	discordHandle: string;
	email: string;
	merchTableOrder: boolean;
};

/** Prefill order form contact info. */
export function writeCheckoutDraft(draft: CheckoutDraft) {
	if (!browser) return;
	sessionStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
}

/** Read then clear a single checkout draft */
export function consumeCheckoutDraft(): CheckoutDraft | null {
	if (!browser) return null;
	try {
		const raw = sessionStorage.getItem(STORAGE_KEY);
		sessionStorage.removeItem(STORAGE_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<CheckoutDraft>;
		return {
			name: String(parsed.name ?? ''),
			discordHandle: String(parsed.discordHandle ?? ''),
			email: String(parsed.email ?? ''),
			merchTableOrder: Boolean(parsed.merchTableOrder)
		};
	} catch {
		sessionStorage.removeItem(STORAGE_KEY);
		return null;
	}
}
