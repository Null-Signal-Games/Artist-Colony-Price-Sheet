// state machine states
export type OrderStatus =
	'new' | 'preparing' | 'prepared' | 'invoiced' | 'notified' | 'paid' | 'closed';

export const ORDER_STATUSES: OrderStatus[] = [
	'new',
	'preparing',
	'prepared',
	'invoiced',
	'notified',
	'paid',
	'closed'
];

export const ORDER_STATUS_LABELS: Record<OrderStatus, string> = {
	new: 'New',
	preparing: 'Preparing',
	prepared: 'Prepared',
	invoiced: 'Invoiced',
	notified: 'Notified',
	paid: 'Paid',
	closed: 'Closed'
};

export type NotificationChannel = 'discord' | 'email' | 'in_person';

export const NOTIFICATION_CHANNEL_LABELS: Record<NotificationChannel, string> = {
	discord: 'Notified On Discord',
	email: 'Notified Via Email Only',
	in_person: 'Notified In Person'
};

export function notificationDisplayLabel(channel: NotificationChannel | null | undefined): string {
	if (channel === 'discord') return NOTIFICATION_CHANNEL_LABELS.discord;
	if (channel === 'email') return NOTIFICATION_CHANNEL_LABELS.email;
	if (channel === 'in_person') return NOTIFICATION_CHANNEL_LABELS.in_person;
	return 'Not Notified Yet';
}

// 'picked_up' -> successfully fulfilled order
// 'closed' -> mark order as closed and not fulfilled
export type ClosedReason = 'picked_up' | 'closed';

export const CLOSED_REASONS: ClosedReason[] = [
	'picked_up',
	'closed',
];

export const CLOSED_REASON_LABELS: Record<ClosedReason, string> = {
	picked_up: 'Picked Up',
	closed: 'Closed'
};

export function orderDisplayLabel(order: {
	status: OrderStatus;
	closedReason?: ClosedReason | null | string;
	preparingBy?: string | null;
}): string {
	if (order.status === 'preparing') {
		const name = (order.preparingBy ?? '').trim();
		return name ? `Preparing by ${name}` : 'Preparing';
	}
	if (order.status === 'closed' && order.closedReason) {
		return (
			CLOSED_REASON_LABELS[order.closedReason as ClosedReason] ??
			(order.closedReason === 'canceled'
				? 'Canceled'
				: order.closedReason === 'refunded'
					? 'Refunded'
					: 'Closed')
		);
	}
	return ORDER_STATUS_LABELS[order.status];
}

export function orderDisplayStatusKey(order: {
	status: OrderStatus;
	closedReason?: ClosedReason | null;
}): string {
	if (order.status === 'closed' && order.closedReason) {
		return order.closedReason;
	}
	return order.status;
}

export type PaidReason = 'shopify' | 'credit_card' | 'paypal' | 'cash';

export const MANUAL_PAID_REASONS: Exclude<PaidReason, 'shopify'>[] = [
	'credit_card',
	'paypal',
	'cash'
];

export const PAID_REASON_LABELS: Record<PaidReason, string> = {
	shopify: 'Paid via Shopify Invoice',
	credit_card: 'Paid via Credit Card',
	paypal: 'Paid via PayPal',
	cash: 'Paid via Cash'
};

export const PAY_ACTION_LABELS: Record<PaidReason, string> = {
	shopify: 'Pay via Shopify Invoice',
	credit_card: 'Pay via Credit Card',
	paypal: 'Pay via PayPal',
	cash: 'Pay via Cash'
};

export type OrderLineAction = '' | 'partial' | 'sold_out';

export type OrderItem = {
	productCode: string;
	title: string;
	artist: string;
	unitPriceCents: number;
	quantity: number; // original order quantity
	lineSubtotalCents: number;
	soldOut: boolean;
	itemType: string;
	collected: boolean;

  // @TODO persist these together for invoicing
	lineAction: OrderLineAction; 
	collectedQuantity: number | null;
};

export function effectiveOrderItemQuantity(item: OrderItem): number {
	if (item.lineAction === 'sold_out') return 0;
	if (item.lineAction === 'partial' && item.collectedQuantity != null) {
		return item.collectedQuantity;
	}
	return item.quantity;
}

export function lineSubtotalForItem(item: OrderItem): number {
	return item.unitPriceCents * effectiveOrderItemQuantity(item);
}

export type Order = {
	orderId: string;
	name: string;
	discordHandle: string;
	email: string;
	subtotalCents: number;
	submittedAt: string;
	createdAt: string;
	status: OrderStatus;
	staffNotes: string;
	items: OrderItem[];
	submittedByStaffName?: string | null;
	preparingBy?: string | null;
	merchTableOrder?: boolean;
	notificationChannel?: NotificationChannel | null;
	shopifyInvoiceId?: string | null;
	paidReason?: PaidReason | null;
	paidReasonOther?: string | null;
	closedReason?: ClosedReason | null;
	closedReasonOther?: string | null;
	history: OrderHistoryEntry[];
	shopifyDraftOrderId?: string | null;
	shopifyInvoiceUrl?: string | null;
	shopifyOrderId?: string | null;
	paidAmountCents?: number | null;
};

export type OrderHistoryKind =
	| 'status'
	| 'paid_reason'
	| 'closed_reason'
	| 'notified'
	| 'invoice'
	| 'notes'
	| 'line';

export type OrderHistoryEntry = {
	id: string;
	at: string;
	staffName: string;
	kind: OrderHistoryKind;
	summary: string;
	fromStatus?: OrderStatus | null;
	toStatus?: OrderStatus | null;
};

// get previous status
export function statusBeforeClose(order: {
	status: OrderStatus;
	closedReason?: ClosedReason | null;
	paidReason?: PaidReason | null;
	history?: OrderHistoryEntry[] | null;
}): OrderStatus | null {
	if (order.status !== 'closed') return null;
	const history = order.history ?? [];
	for (let i = history.length - 1; i >= 0; i--) {
		const entry = history[i];
		if (entry.kind === 'status' && entry.toStatus === 'closed' && entry.fromStatus) {
			return entry.fromStatus;
		}
	}
	if (order.closedReason === 'picked_up' || order.paidReason) return 'paid';
	return 'new';
}

export type StaffActionMeta = {
	staffName?: string;
};

export type SendInvoiceResult = {
	order: Order;
	invoiceId: string;
	message: string;
	invoiceUrl?: string;
};

export type OrderSummary = Omit<Order, 'items' | 'history'>;

export type OrderSort = 'oldest' | 'newest';

export type ListOrdersFilter = {
	statuses?: OrderStatus[];
	q?: string;
	sort?: OrderSort;
};

export type UpdateOrderStatusOptions = StaffActionMeta & {
	closedReason?: ClosedReason;
	closedReasonOther?: string;
	paidReason?: PaidReason;
	paidReasonOther?: string;
	notificationChannel?: NotificationChannel;
	paidAmountCents?: number;
};

export type InventoryItem = {
	id: string;
	shopName: string;
	artistName: string;
	productCode: string;
	title: string;
	itemType: string;
	productDisplay: string;
	quantity: number | null;
	priceCents: number;
	notes: string;
	soldOut: boolean;
};

export type ListInventoryFilter = {
	q?: string;
	shop?: string;
};

export type CreateStaffOrderInput = {
	orderId: string;
	name: string;
	discordHandle: string;
	email: string;
	items: Array<{
		productCode: string;
		title: string;
		artist: string;
		unitPriceCents: number;
		quantity: number;
	}>;
	submittedByStaffName: string;
	merchTableOrder?: boolean;
};

export type StaffApi = {
	listOrders(filter?: ListOrdersFilter): Promise<OrderSummary[]>;
	getOrder(orderId: string): Promise<Order | null>;
	createStaffOrder(input: CreateStaffOrderInput, meta?: StaffActionMeta): Promise<Order>;
	updateOrderStatus(
		orderId: string,
		status: OrderStatus,
		options?: UpdateOrderStatusOptions
	): Promise<Order>;
	updateOrderNotes(
		orderId: string,
		staffNotes: string,
		meta?: StaffActionMeta
	): Promise<Order>;

  // @TODO wire up api to support this
	updateOrderLine(
		orderId: string,
		lineIndex: number,
		patch: Partial<Pick<OrderItem, 'collected' | 'lineAction' | 'collectedQuantity'>>,
		meta?: StaffActionMeta
	): Promise<Order>;

  // @TODO WIP, stubbed shopify invoice action
	sendInvoice(orderId: string, meta?: StaffActionMeta): Promise<SendInvoiceResult>;

	// @TODO WIP stubbed api
	resendInvoice(orderId: string, meta?: StaffActionMeta): Promise<SendInvoiceResult>;

	markNotified(
		orderId: string,
		channel: NotificationChannel,
		meta?: StaffActionMeta
	): Promise<Order>;
	listInventory(filter?: ListInventoryFilter): Promise<InventoryItem[]>;
	setProductSoldOut(id: string, soldOut: boolean): Promise<InventoryItem>;
	subscribeToOrderEvents(onEvent: () => void, onError: (err: any) => void): () => void;
};
