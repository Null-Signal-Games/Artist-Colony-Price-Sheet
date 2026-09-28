import { apiBaseUrl, getStaffToken } from './staffIdentity';
import type {
	CreateStaffOrderInput,
	InventoryItem,
	ListInventoryFilter,
	ListOrdersFilter,
	NotificationChannel,
	Order,
	OrderLineAction,
	OrderStatus,
	OrderSummary,
	SendInvoiceResult,
	StaffActionMeta,
	StaffApi,
	UpdateOrderStatusOptions
} from './types';

export class StaffAuthError extends Error {
	constructor(message = 'Session expired. Sign in again.') {
		super(message);
		this.name = 'StaffAuthError';
	}
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const token = getStaffToken();
	if (!token) throw new StaffAuthError();

	const res = await fetch(`${apiBaseUrl()}${path}`, {
		...init,
		headers: {
			Authorization: `Bearer ${token}`,
			...(init.body ? { 'Content-Type': 'application/json' } : {}),
			...init.headers
		}
	});

	if (res.status === 401) throw new StaffAuthError();

	const body = await res.json().catch(() => ({}));
	if (!res.ok) {
		throw new Error(body.error || `Request failed (${res.status}).`);
	}
	return body as T;
}

function ordersQuery(filter: ListOrdersFilter) {
	const params = new URLSearchParams();
	if (filter.statuses?.length) params.set('statuses', filter.statuses.join(','));
	if (filter.q?.trim()) params.set('q', filter.q.trim());
	if (filter.sort) params.set('sort', filter.sort);
	const qs = params.toString();
	return qs ? `?${qs}` : '';
}

export function createRemoteStaffApi(): StaffApi {
	return {
		subscribeToOrderEvents(onEvent: () => void, onError: (err: any) => void): () => void {
			const token = getStaffToken();
			if (!token) {
				onError(new StaffAuthError());
				return () => {};
			}

			const es = new EventSource(`${apiBaseUrl()}/staff/orders/events?token=${encodeURIComponent(token)}`);
			es.onmessage = (event) => {
				if (event.data !== 'ping') {
					onEvent();
				}
			};
			es.onerror = (err) => {
				es.close();
				onError(err);
			};

			return () => es.close();
		},

		async listOrders(filter: ListOrdersFilter = {}): Promise<OrderSummary[]> {
			const list = await request<OrderSummary[]>(`/staff/orders${ordersQuery(filter)}`);
			return list ?? [];
		},

		async getOrder(orderId: string): Promise<Order | null> {
			const token = getStaffToken();
			if (!token) throw new StaffAuthError();
			const res = await fetch(`${apiBaseUrl()}/staff/orders/${encodeURIComponent(orderId)}`, {
				headers: { Authorization: `Bearer ${token}` }
			});
			if (res.status === 404) return null;
			if (res.status === 401) throw new StaffAuthError();
			const body = await res.json().catch(() => ({}));
			if (!res.ok) throw new Error(body.error || `Request failed (${res.status}).`);
			return body as Order;
		},

		async createStaffOrder(input: CreateStaffOrderInput, _meta?: StaffActionMeta) {
			return request<Order>('/staff/orders', {
				method: 'POST',
				body: JSON.stringify(input)
			});
		},

		async updateOrderStatus(
			orderId: string,
			status: OrderStatus,
			options: UpdateOrderStatusOptions = {}
		) {
			const { staffName: _staffName, ...patch } = options;
			return request<Order>(`/staff/orders/${encodeURIComponent(orderId)}/status`, {
				method: 'POST',
				body: JSON.stringify({ status, ...patch })
			});
		},

		async updateOrderNotes(orderId: string, staffNotes: string, _meta?: StaffActionMeta) {
			return request<Order>(`/staff/orders/${encodeURIComponent(orderId)}/notes`, {
				method: 'PATCH',
				body: JSON.stringify({ staffNotes })
			});
		},

		async updateOrderLine(
			orderId: string,
			lineIndex: number,
			patch: Partial<Pick<
				{ collected: boolean; lineAction: OrderLineAction; collectedQuantity: number | null },
				'collected' | 'lineAction' | 'collectedQuantity'
			>>,
			_meta?: StaffActionMeta
		) {
			return request<Order>(
				`/staff/orders/${encodeURIComponent(orderId)}/lines/${lineIndex}`,
				{ method: 'PATCH', body: JSON.stringify(patch) }
			);
		},

		async sendInvoice(orderId: string, _meta?: StaffActionMeta): Promise<SendInvoiceResult> {
			return request<SendInvoiceResult>(
				`/staff/orders/${encodeURIComponent(orderId)}/send-invoice`,
				{ method: 'POST' }
			);
		},

		async resendInvoice(orderId: string, _meta?: StaffActionMeta): Promise<SendInvoiceResult> {
			return request<SendInvoiceResult>(
				`/staff/orders/${encodeURIComponent(orderId)}/resend-invoice`,
				{ method: 'POST' }
			);
		},

		async markNotified(
			orderId: string,
			channel: NotificationChannel,
			_meta?: StaffActionMeta
		): Promise<Order> {
			return request<Order>(`/staff/orders/${encodeURIComponent(orderId)}/notified`, {
				method: 'POST',
				body: JSON.stringify({ channel })
			});
		},

		async listInventory(filter: ListInventoryFilter = {}): Promise<InventoryItem[]> {
			const params = new URLSearchParams();
			if (filter.q?.trim()) params.set('q', filter.q.trim());
			if (filter.shop?.trim()) params.set('shop', filter.shop.trim());
			const qs = params.toString();
			const list = await request<InventoryItem[]>(`/staff/inventory${qs ? `?${qs}` : ''}`);
			return list ?? [];
		},

		async setProductSoldOut(id: string, soldOut: boolean): Promise<InventoryItem> {
			return request<InventoryItem>(
				`/staff/inventory/${encodeURIComponent(id)}/sold-out`,
				{ method: 'PATCH', body: JSON.stringify({ soldOut }) }
			);
		}
	};
}
