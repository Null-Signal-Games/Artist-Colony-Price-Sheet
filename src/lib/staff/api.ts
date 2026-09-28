import { createRemoteStaffApi } from './remoteApi';
import type { StaffApi } from './types';

export { StaffAuthError } from './remoteApi';
export { apiBaseUrl, loginStaff, logoutStaff } from './staffIdentity';

export type {
	ClosedReason,
	CreateStaffOrderInput,
	InventoryItem,
	ListInventoryFilter,
	ListOrdersFilter,
	Order,
	OrderHistoryEntry,
	OrderItem,
	OrderLineAction,
	OrderSort,
	OrderStatus,
	OrderSummary,
	NotificationChannel,
	PaidReason,
	SendInvoiceResult,
	StaffActionMeta,
	StaffApi,
	UpdateOrderStatusOptions
} from './types';

export {
	CLOSED_REASONS,
	CLOSED_REASON_LABELS,
	MANUAL_PAID_REASONS,
	PAID_REASON_LABELS,
	PAY_ACTION_LABELS,
	ORDER_STATUSES,
	ORDER_STATUS_LABELS,
	NOTIFICATION_CHANNEL_LABELS,
	notificationDisplayLabel,
	orderDisplayLabel,
	orderDisplayStatusKey,
	effectiveOrderItemQuantity,
	lineSubtotalForItem,
	statusBeforeClose
} from './types';
export { inventoryItemId } from './catalog';
export {
	getCurrentStaffName,
	setCurrentStaffName,
	isStaffSession,
	enableStaffSession,
	clearStaffSession,
	syncStaffSessionClass,
	getStaffToken
} from './staffIdentity';

export const staffApi: StaffApi = createRemoteStaffApi();

export function formatCents(cents: number) {
	return `$${(cents / 100).toFixed(2)}`;
}
