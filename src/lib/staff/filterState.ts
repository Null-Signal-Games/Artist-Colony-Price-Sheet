import { ORDER_STATUSES, type OrderSort, type OrderStatus } from './types';

const STORAGE_KEY = 'artist-colony-staff-filters-v4';

export type StaffFilterState = {
	statusChecks: Record<OrderStatus, boolean>;
	orderQuery: string;
	orderSort: OrderSort;
	inventoryQuery: string;
};

export function defaultStatusChecks(enabled = true): Record<OrderStatus, boolean> {
	return Object.fromEntries(ORDER_STATUSES.map((status) => [status, enabled])) as Record<
		OrderStatus,
		boolean
	>;
}

export function defaultFilterState(): StaffFilterState {
	return {
		statusChecks: defaultStatusChecks(true),
		orderQuery: '',
		orderSort: 'oldest',
		inventoryQuery: ''
	};
}

function isOrderSort(value: unknown): value is OrderSort {
	return value === 'oldest' || value === 'newest';
}

export function readStoredFilterState(): StaffFilterState {
	const defaults = defaultFilterState();
	if (typeof localStorage === 'undefined') return defaults;

	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return defaults;
		const parsed = JSON.parse(raw) as Partial<StaffFilterState>;
		const checks = defaultStatusChecks(true);
		if (parsed.statusChecks && typeof parsed.statusChecks === 'object') {
			for (const status of ORDER_STATUSES) {
				if (typeof parsed.statusChecks[status] === 'boolean') {
					checks[status] = parsed.statusChecks[status];
				}
			}
		}
		return {
			statusChecks: checks,
			orderQuery: typeof parsed.orderQuery === 'string' ? parsed.orderQuery : '',
			orderSort: isOrderSort(parsed.orderSort) ? parsed.orderSort : 'oldest',
			inventoryQuery: typeof parsed.inventoryQuery === 'string' ? parsed.inventoryQuery : ''
		};
	} catch {
		return defaults;
	}
}

export function writeStoredFilterState(state: StaffFilterState) {
	if (typeof localStorage === 'undefined') return;
	try {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
	} catch {
		// noop
	}
}
