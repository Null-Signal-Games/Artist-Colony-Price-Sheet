import { env } from '$env/dynamic/public';
import { formatMoney, parseMoney, type CartItem } from '$lib/cart';

const DEFAULT_ORDER_ENDPOINT = 'https://artist-colony.netrunner-meetup.com';
const ORDER_PATH = '/order';

export type OrderSubmission = {
	orderId: string;
	name: string;
	discordHandle: string;
	email: string;
	items: CartItem[];
	subtotal: string;
	submittedAt: string;
};

export type OrderSubmitResult = { ok: true; orderId: string } | { ok: false; error: string };

function endpointBase() {
	const configured = env.PUBLIC_ORDER_ENDPOINT?.trim();
	return (configured || DEFAULT_ORDER_ENDPOINT).replace(/\/+$/, '');
}

export function buildOrderId(date = new Date()) {
	return `W26-${date.getTime().toString().slice(-8)}`;
}

export function buildSubtotal(items: CartItem[]) {
	const total = items.reduce((sum, item) => sum + parseMoney(item.cad) * item.quantity, 0);
	return formatMoney(total, '$');
}

export async function submitOrder(order: OrderSubmission): Promise<OrderSubmitResult> {
	const endpoint = `${endpointBase()}${ORDER_PATH}`;

	try {
		const response = await fetch(endpoint, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(order)
		});

		const text = await response.text();
		let data: { result?: string; orderId?: string; error?: string } = {};

		try {
			data = text ? JSON.parse(text) : {};
		} catch {
			if (!response.ok) {
				return { ok: false, error: `Order server returned ${response.status}.` };
			}
		}

		if (!response.ok || data.result === 'error') {
			return {
				ok: false,
				error: data.error || `Order server returned ${response.status}.`
			};
		}

		return { ok: true, orderId: data.orderId || order.orderId };
	} catch (error) {
		return {
			ok: false,
			error: error instanceof Error ? error.message : 'Could not reach the order server.'
		};
	}
}
