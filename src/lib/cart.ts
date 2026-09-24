import { browser } from '$app/environment';
import { derived, writable } from 'svelte/store';

export type CartItem = {
	id: string;
	productCode: string;
	title: string;
	artist: string;
	cad: string;
	quantity: number;
};

const STORAGE_KEY = 'artist-colony-cart-v2';

function readStoredCart(): CartItem[] {
	if (!browser) return [];

	try {
		const stored = localStorage.getItem(STORAGE_KEY);
		return stored ? (JSON.parse(stored) as CartItem[]) : [];
	} catch {
		return [];
	}
}

function createCart() {
	const { subscribe, set, update } = writable<CartItem[]>(readStoredCart());

	if (browser) {
		subscribe((items) => {
			localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
		});
	}

	return {
		subscribe,
		add(item: Omit<CartItem, 'quantity' | 'id'> & { id?: string }) {
			const id = item.id ?? cartItemId(item);

			update((items) => {
				const existing = items.find((entry) => entry.id === id);
				if (existing) {
					return items.map((entry) =>
						entry.id === id ? { ...entry, quantity: entry.quantity + 1 } : entry
					);
				}

				return [...items, { ...item, id, quantity: 1 }];
			});
		},
		setQuantity(id: string, quantity: number) {
			update((items) => {
				if (quantity <= 0) {
					return items.filter((entry) => entry.id !== id);
				}

				return items.map((entry) => (entry.id === id ? { ...entry, quantity } : entry));
			});
		},
		remove(id: string) {
			update((items) => items.filter((entry) => entry.id !== id));
		},
		clear() {
			set([]);
		}
	};
}

export function cartItemId(item: { productCode: string; title: string }) {
	return `${item.productCode}::${item.title}`;
}

export function parseMoney(value: string): number {
	const amount = parseFloat(String(value).replace(/[^0-9.-]/g, ''));
	return Number.isFinite(amount) ? amount : 0;
}

export function formatMoney(amount: number, symbol: string): string {
	return `${symbol}${amount.toFixed(2)}`;
}

export const cart = createCart();

export const cartCount = derived(cart, (items) =>
	items.reduce((total, item) => total + item.quantity, 0)
);
