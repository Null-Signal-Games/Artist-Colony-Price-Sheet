<script lang="ts">
	import { goto, replaceState } from '$app/navigation';
	import { base } from '$app/paths';
	import { page } from '$app/stores';
	import { onMount, tick } from 'svelte';
	import { get } from 'svelte/store';
	import { env } from '$env/dynamic/public';

	import {
		effectiveOrderItemQuantity,
		formatCents,
		MANUAL_PAID_REASONS,
		ORDER_STATUS_LABELS,
		ORDER_STATUSES,
		PAID_REASON_LABELS,
		PAY_ACTION_LABELS,
		orderDisplayLabel,
		orderDisplayStatusKey,
		enableStaffSession,
		getCurrentStaffName,
		getStaffToken,
		loginStaff,
		logoutStaff,
		StaffAuthError,
		staffApi,
		type ClosedReason,
		type InventoryItem,
		type Order,
		type OrderItem,
		type OrderLineAction,
		type OrderSort,
		type OrderStatus,
		type OrderSummary,
		type PaidReason,
		type NotificationChannel,
	} from '$lib/staff/api';
	import {
		defaultStatusChecks,
		readStoredFilterState,
		writeStoredFilterState
	} from '$lib/staff/filterState';
	import {
		cart,
		cartItemId,
		formatMoney
	} from '$lib/cart';
	import { writeCheckoutDraft } from '$lib/checkoutDraft';
	import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';

	let authed = false;
	let authChecked = false;
	let loginUsername = '';
	let loginPassword = '';
	let loginError = '';
	let loginBusy = false;

	type Tab = 'orders' | 'inventory';

	let tab: Tab = 'orders';
	let loading = true;
	let saving = false;
	let error = '';
	let syncNotice = '';
	let syncRefreshing = false;

	let statusChecks = defaultStatusChecks(true);
	let orderSort: OrderSort = 'oldest';
	let orderQuery = '';
	let orders: OrderSummary[] = [];
	let selectedOrderId = '';
	let selectedOrder: Order | null = null;
	let notesDraft = '';

	let paidMenuOpen = false;
	let statusMenuOpen = false;
	let prepareConfirmOpen = false;
	let invoicedConfirmOpen = false;
	let cancelConfirmOpen = false;
	let paidAmountConfirmOpen = false;
	let paidAmountDraft = '';
	let paidAmountInputEl: HTMLInputElement | null = null;
	let pendingPaidReason: Exclude<PaidReason, 'shopify'> | null = null;
	let discordNotifyOpen = false;

	let inventoryQuery = '';
	let inventory: InventoryItem[] = [];
	let savingSoldOutId = '';
	let soldOutMenuItemId = '';

	let menuOpen = false;
	let statusFilterOpen = false;
	let searchOpen = false;
	let searchInputEl: HTMLInputElement | null = null;
	let openLineMenuIndex: number | null = null;
	let sendingInvoice = false;
	let invoiceSuccess = '';
	let copyFlash = '';
	let partialQtyEditingIndex: number | null = null;
	let partialQtyDraft = '';
	let partialQtyInputEl: HTMLInputElement | null = null;

	let sseCleanup: (() => void) | null = null;
	let pollInterval: ReturnType<typeof setInterval> | null = null;

	$: if (authed && tab === 'orders') {
		setupOrderSync();
	} else {
		cleanupOrderSync();
	}

	// filters
	let appliedStatusLimited = false;
	let appliedOrderQuery = '';
	let appliedOrderSort: OrderSort = 'oldest';
	let appliedInventoryQuery = '';

	$: actionsLocked = saving || sendingInvoice;

	function syncSelectedOrderUrl(orderId: string) {
		const url = new URL(get(page).url);
		if (orderId) {
			url.searchParams.set('order', orderId);
		} else {
			url.searchParams.delete('order');
		}
		const next = `${url.pathname}${url.search}${url.hash}`;
		const current = `${get(page).url.pathname}${get(page).url.search}${get(page).url.hash}`;
		if (next !== current) {
			replaceState(next, {});
		}
	}

	$: lineItemsHardLocked =
		!!selectedOrder &&
		(selectedOrder.status === 'invoiced' ||
			selectedOrder.status === 'notified' ||
			selectedOrder.status === 'paid' ||
			selectedOrder.status === 'closed');

	$: lineItemsSoftLocked = !!selectedOrder && selectedOrder.status === 'prepared';

	$: lineItemsEditable = !lineItemsHardLocked && !lineItemsSoftLocked;

	$: hideBarFilters = tab === 'orders' && !!selectedOrder;

	$: filteredStatuses = ORDER_STATUSES.filter((status) => statusChecks[status]);

	$: statusFilterLabel =
		filteredStatuses.length === ORDER_STATUSES.length
			? 'All Orders'
			: filteredStatuses.length === 0
				? 'No Filters'
				: filteredStatuses.length === 1
					? ORDER_STATUS_LABELS[filteredStatuses[0]]
					: `${filteredStatuses.length} Filters`;

	function staffMeta() {
		return { staffName: getCurrentStaffName() };
	}

	function selectedStatuses(): OrderStatus[] {
		return filteredStatuses;
	}

	function allStatusesSelected() {
		return filteredStatuses.length === ORDER_STATUSES.length;
	}

	async function onStatusFilterChange() {
		statusChecks = { ...statusChecks };
		await loadOrders();
	}

	async function showAllStatuses() {
		statusChecks = defaultStatusChecks(true);
		await loadOrders();
	}

	function persistFilters() {
		writeStoredFilterState({
			statusChecks,
			orderQuery,
			orderSort,
			inventoryQuery
		});
	}

	function orderListFilter() {
		return {
			statuses: allStatusesSelected() ? undefined : selectedStatuses(),
			q: orderQuery,
			sort: orderSort
		};
	}

	async function toggleOrderSort() {
		orderSort = orderSort === 'oldest' ? 'newest' : 'oldest';
		await loadOrders();
	}

	async function loadOrders(background = false) {
		if (!background) loading = true;
		error = '';
		try {
			orders = await staffApi.listOrders(orderListFilter());
			appliedStatusLimited = !allStatusesSelected();
			appliedOrderQuery = orderQuery.trim();
			appliedOrderSort = orderSort;
			persistFilters();
			// keeps the active order detail open even if filters no longer include it in the list
			if (selectedOrderId) {
				await flushNotesIfNeeded({ quiet: true });
				selectedOrder = await staffApi.getOrder(selectedOrderId);
				notesDraft = selectedOrder?.staffNotes ?? '';
				if (!selectedOrder) {
					await clearSelectedOrder({ skipFlush: true });
				}
			}
		} catch (err) {
			if (err instanceof StaffAuthError) {
				authed = false;
				loginError = err.message;
			} else {
				if (!background) error = err instanceof Error ? err.message : 'Failed to load orders.';
			}
		} finally {
			loading = false;
		}
	}

	function cleanupOrderSync() {
		if (sseCleanup) {
			sseCleanup();
			sseCleanup = null;
		}
		if (pollInterval) {
			clearInterval(pollInterval);
			pollInterval = null;
		}
	}

	function setupOrderSync() {
		cleanupOrderSync();
		sseCleanup = staffApi.subscribeToOrderEvents(
			() => {
				loadOrders(true);
			},
			(err: any) => {
				console.error('SSE failed, falling back to polling', err);
				cleanupOrderSync();
				pollInterval = setInterval(() => {
					loadOrders(true);
				}, 5000);
			}
		);
	}

	async function refreshOrdersList() {
		orders = await staffApi.listOrders(orderListFilter());
	}

	async function loadInventory() {
		loading = true;
		error = '';
		try {
			inventory = await staffApi.listInventory({ q: inventoryQuery });
			appliedInventoryQuery = inventoryQuery.trim();
			persistFilters();
		} catch (err) {
			if (err instanceof StaffAuthError) {
				authed = false;
				loginError = err.message;
			} else {
				error = err instanceof Error ? err.message : 'Failed to load inventory.';
			}
		} finally {
			loading = false;
		}
	}

	async function flushNotesIfNeeded(options: { quiet?: boolean } = {}) {
		if (!selectedOrder) return true;
		const saved = selectedOrder.staffNotes ?? '';
		if (notesDraft === saved) return true;
		const orderId = selectedOrder.orderId;
		const notes = notesDraft;
		if (!options.quiet) saving = true;
		error = '';
		try {
			const updated = await staffApi.updateOrderNotes(orderId, notes, staffMeta());
			if (selectedOrder?.orderId === orderId) {
				selectedOrder = updated;
				notesDraft = updated.staffNotes ?? notes;
			}
			await refreshOrdersList();
			return true;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to save notes.';
			return false;
		} finally {
			if (!options.quiet) saving = false;
		}
	}

	async function selectOrder(orderId: string) {
		if (selectedOrderId && selectedOrderId !== orderId) {
			await flushNotesIfNeeded({ quiet: true });
		}
		selectedOrderId = orderId;
		syncSelectedOrderUrl(orderId);
		invoiceSuccess = '';
		openLineMenuIndex = null;
		prepareConfirmOpen = false;
		invoicedConfirmOpen = false;
		statusFilterOpen = false;
		searchOpen = false;
		saving = true;
		error = '';
		cancelConfirmOpen = false;
		paidAmountConfirmOpen = false;
		pendingPaidReason = null;
		paidAmountDraft = '';
		partialQtyEditingIndex = null;
		partialQtyDraft = '';
		discordNotifyOpen = false;
		try {
			selectedOrder = await staffApi.getOrder(orderId);
			if (!selectedOrder) {
				await clearSelectedOrder({ skipFlush: true });
				error = 'Order not found.';
				return;
			}
			notesDraft = selectedOrder.staffNotes ?? '';
			paidMenuOpen = false;
			statusMenuOpen = false;
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to load order.';
		} finally {
			saving = false;
		}
	}

	async function clearSelectedOrder(options: { skipFlush?: boolean } = {}) {
		if (!options.skipFlush) {
			await flushNotesIfNeeded({ quiet: true });
		}
		selectedOrderId = '';
		selectedOrder = null;
		notesDraft = '';
		paidMenuOpen = false;
		statusMenuOpen = false;
		prepareConfirmOpen = false;
		invoicedConfirmOpen = false;
		openLineMenuIndex = null;
		invoiceSuccess = '';
		statusFilterOpen = false;
		searchOpen = false;
		syncSelectedOrderUrl('');
		cancelConfirmOpen = false;
		paidAmountConfirmOpen = false;
		pendingPaidReason = null;
		paidAmountDraft = '';
		partialQtyEditingIndex = null;
		partialQtyDraft = '';
		discordNotifyOpen = false;
		error = '';
	}

	async function setStatus(
		status: OrderStatus,
		options: {
			closedReason?: ClosedReason;
			paidReason?: PaidReason;
			notificationChannel?: NotificationChannel;
			paidAmountCents?: number;
		} = {}
	) {
		if (!selectedOrder || actionsLocked) return;
		if (!canSelectStatus(status) && status !== selectedOrder.status) return;
		if (!(await flushNotesIfNeeded({ quiet: true }))) return;
		saving = true;
		error = '';
		invoiceSuccess = '';
		const orderId = selectedOrder.orderId;
		try {
			const closedReason = options.closedReason ?? 'closed';
			const paidReason = options.paidReason ?? 'cash';
			const meta = staffMeta();
			selectedOrder = await staffApi.updateOrderStatus(
				orderId,
				status,
				status === 'closed'
					? { ...meta, closedReason }
					: status === 'paid'
						? {
								...meta,
								paidReason,
								paidAmountCents:
									options.paidAmountCents ?? collectedTotalCents(selectedOrder)
						  }
						: status === 'notified'
							? {
									...meta,
									notificationChannel:
										options.notificationChannel ??
										selectedOrder.notificationChannel ??
										undefined
								}
								: meta
			);
			await refreshOrdersList();
		} catch (err) {
			const message = err instanceof Error ? err.message : 'Failed to update status.';
			if (status === 'paid') {
				try {
					const refreshed = await staffApi.getOrder(orderId);
					if (refreshed) {
						selectedOrder = refreshed;
						notesDraft = refreshed.staffNotes ?? notesDraft;
						await refreshOrdersList();
					}
					if (selectedOrder?.status === 'paid') {
						error = '';
						return;
					}
				} catch {
					/* keep previous selectedOrder */
				}
			}
			error = message;
		} finally {
			saving = false;
		}
	}

	async function duplicateOrder() {
		if (!selectedOrder || actionsLocked) return;
		closeStatusMenu();
		closePaidMenu();
		const source = selectedOrder;
		cart.replace(
			source.items.map((item) => ({
				id: cartItemId(item),
				productCode: item.productCode,
				title: item.title,
				artist: item.artist,
				cad: formatMoney(item.unitPriceCents / 100, ''),
				quantity: item.quantity
			}))
		);
		writeCheckoutDraft({
			name: source.name,
			discordHandle: source.discordHandle,
			email: source.email,
			merchTableOrder: Boolean(source.merchTableOrder)
		});
		await goto(`${base}/order-form`);
	}

	/* async function saveClosedReasonOther() {
		if (!selectedOrder || selectedOrder.status !== 'closed' || actionsLocked) return;
		if (selectedOrder.closedReason !== 'other') return;
		await setClosedReason('other');
	} */

	async function markPaid(
		reason: Exclude<PaidReason, 'shopify'>,
		paidAmountCents?: number
	): Promise<boolean> {
		if (!selectedOrder || actionsLocked) return false;
		if (selectedOrder.status !== 'notified') return false;
		error = '';
		const amountCents = paidAmountCents ?? collectedTotalCents(selectedOrder);
		await setStatus('paid', {
			paidReason: reason,
			paidAmountCents: amountCents
		});
		if (selectedOrder?.status === 'paid') {
			const method =
				reason === 'credit_card' ? 'Credit Card' : reason === 'paypal' ? 'PayPal' : 'Cash';
			const paidNote = `Paid ${formatCents(amountCents)} via ${method}`;
			const existing = (notesDraft || selectedOrder.staffNotes || '').trim();
			notesDraft = existing ? `${existing}\n${paidNote}` : paidNote;
			await flushNotesIfNeeded({ quiet: true });
			closePaidMenu();
			closeStatusMenu();
			return true;
		}
		closePaidMenu();
		closeStatusMenu();
		return false;
	}

	function parsePaidAmountToCents(raw: string): number | null {
		const cleaned = raw.trim().replace(/[$,\s]/g, '');
		if (!cleaned || !/^\d+(\.\d{0,2})?$/.test(cleaned)) return null;
		const dollars = Number(cleaned);
		if (!Number.isFinite(dollars) || dollars <= 0) return null;
		return Math.round(dollars * 100);
	}

	function paidConfirmTitle(reason: Exclude<PaidReason, 'shopify'>) {
		if (reason === 'credit_card') return 'Pay via Credit Card';
		if (reason === 'paypal') return 'Pay via PayPal';
		return 'Pay via Cash';
	}

	function paidConfirmDetail(reason: Exclude<PaidReason, 'shopify'>) {
		if (reason === 'credit_card') return 'Enter the amount received via credit card for this order.';
		if (reason === 'paypal') return 'Enter the amount received via PayPal for this order.';
		return 'Enter the amount of cash received for this order.';
	}

	function paidConfirmActionLabel(reason: Exclude<PaidReason, 'shopify'>) {
		return PAID_REASON_LABELS[reason].replace(/^Paid/, 'Mark Paid');
	}

	async function requestManualPaid(reason: Exclude<PaidReason, 'shopify'>) {
		if (!selectedOrder || selectedOrder.status !== 'notified' || actionsLocked) return;
		closeStatusMenu();
		closePaidMenu();
		error = '';
		pendingPaidReason = reason;
		const suggested = collectedTotalCents(selectedOrder);
		paidAmountDraft = (suggested / 100).toFixed(2);
		paidAmountConfirmOpen = true;
		await tick();
		paidAmountInputEl?.focus();
		paidAmountInputEl?.select();
	}

	function closePaidAmountConfirm() {
		if (actionsLocked) return;
		paidAmountConfirmOpen = false;
		pendingPaidReason = null;
		paidAmountDraft = '';
	}

	async function confirmManualPaid() {
		if (!selectedOrder || selectedOrder.status !== 'notified' || actionsLocked) return;
		if (!pendingPaidReason) return;
		const cents = parsePaidAmountToCents(paidAmountDraft);
		if (cents == null) {
			error = 'Enter a valid amount received.';
			return;
		}
		const reason = pendingPaidReason;
		await markPaid(reason, cents);
		paidAmountConfirmOpen = false;
		pendingPaidReason = null;
		paidAmountDraft = '';
	}

	async function refreshOrderAfterSync() {
		if (!selectedOrderId || syncRefreshing) return;
		syncRefreshing = true;
		error = '';
		try {
			const refreshed = await staffApi.getOrder(selectedOrderId);
			if (refreshed) {
				selectedOrder = refreshed;
				notesDraft = refreshed.staffNotes ?? '';
			}
			await refreshOrdersList();
			if (selectedOrder?.status === 'paid') {
				syncNotice = '';
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to refresh order.';
		} finally {
			syncRefreshing = false;
		}
	}

	async function choosePaidReason(reason: PaidReason) {
		if (reason === 'shopify' || actionsLocked) return;
		closeStatusMenu();
		if (selectedOrder?.status !== 'notified') return;
		await requestManualPaid(reason);
	}

	function closePaidMenu() {
		paidMenuOpen = false;
	}

	function closeStatusMenu() {
		statusMenuOpen = false;
	}

	function togglePaidMenu() {
		if (actionsLocked || !selectedOrder || selectedOrder.status !== 'paid') return;
		statusMenuOpen = false;
		paidMenuOpen = !paidMenuOpen;
	}

	function toggleStatusMenu() {
		if (actionsLocked || !selectedOrder) return;
		const isClosedMenu =
			selectedOrder.status === 'closed' &&
			(selectedOrder.closedReason === 'picked_up' ||
				selectedOrder.closedReason === 'closed');
		if (
			selectedOrder.status !== 'new' &&
			selectedOrder.status !== 'prepared' &&
			selectedOrder.status !== 'invoiced' &&
			selectedOrder.status !== 'notified' &&
			!isClosedMenu
		) {
			return;
		}
		paidMenuOpen = false;
		statusMenuOpen = !statusMenuOpen;
	}

	function requestCancelOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status === 'closed') return;
		closeStatusMenu();
		closePaidMenu();
		cancelConfirmOpen = true;
	}

	function closeCancelConfirm() {
		cancelConfirmOpen = false;
	}

	async function confirmCancelOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status === 'closed') return;
		cancelConfirmOpen = false;
		await setStatus('closed', { closedReason: 'closed' });
	}

	async function refundOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status === 'closed') return;
		closeStatusMenu();
		closePaidMenu();
		await setStatus('closed', { closedReason: 'refunded' });
	}

	async function sendInvoiceFromMenu() {
		closeStatusMenu();
		await sendInvoice();
	}

	async function resendInvoiceFromMenu() {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status !== 'invoiced' && selectedOrder.status !== 'notified') return;
		closeStatusMenu();
		sendingInvoice = true;
		error = '';
		invoiceSuccess = '';
		try {
			const result = await staffApi.resendInvoice(selectedOrder.orderId, staffMeta());
			selectedOrder = result.order;
			invoiceSuccess = result.message;
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to resend invoice.';
		} finally {
			sendingInvoice = false;
		}
	}

	function isLineReadyForPrepare(item: OrderItem) {
		return item.lineAction === 'sold_out' || item.collected;
	}

	/* function orderWasPaid(order: Order) {
		return order.status === 'paid' || Boolean(order.paidReason);
	}
  */

	function collectedTotalCents(order: Order) {
		return order.items.reduce(
			(sum, item) => sum + effectiveOrderItemQuantity(item) * item.unitPriceCents,
			0
		);
	}

	$: allLinesReadyForPrepare = selectedOrder
		? selectedOrder.items.length > 0 && selectedOrder.items.every(isLineReadyForPrepare)
		: false;

	$: paidViaShopify = !!selectedOrder && selectedOrder.paidReason === 'shopify';

	$: paidMethodLabel = selectedOrder?.paidReason
		? PAID_REASON_LABELS[selectedOrder.paidReason]
		: 'Paid';

	$: paidAmountLabel =
		selectedOrder?.paidAmountCents != null
			? formatCents(selectedOrder.paidAmountCents)
			: null;

	// use subdomain for links to shopify admin
	const SHOPIFY_STORE_ID = (env.PUBLIC_SHOPIFY_STORE_SUBDOMAIN ?? '').trim();

	function shopifyGidId(gid: string | null | undefined) {
		if (!gid) return '';
		const parts = gid.split('/');
		return parts[parts.length - 1] ?? '';
	}

	$: shopifyOrderLink =
		SHOPIFY_STORE_ID && selectedOrder?.shopifyOrderId
			? `https://admin.shopify.com/store/${SHOPIFY_STORE_ID}/orders/${shopifyGidId(selectedOrder.shopifyOrderId)}`
			: null;

	$: shopifyDraftLink =
		SHOPIFY_STORE_ID && selectedOrder?.shopifyDraftOrderId
			? `https://admin.shopify.com/store/${SHOPIFY_STORE_ID}/draft_orders/${shopifyGidId(selectedOrder.shopifyDraftOrderId)}`
			: null;

	$: canReopenClosed =
		!!selectedOrder &&
		selectedOrder.status === 'closed' &&
		!!statusBeforeClose(selectedOrder);

	$: isPickedUp =
		!!selectedOrder &&
		selectedOrder.status === 'closed' &&
		selectedOrder.closedReason === 'picked_up';

	$: statusHistoryEntries = (selectedOrder?.history ?? []).filter(
		(entry) =>
			entry.kind === 'status' ||
			entry.kind === 'invoice' ||
			entry.kind === 'notified' ||
			entry.kind === 'paid_reason' ||
			entry.kind === 'closed_reason'
	);

	function canSelectStatus(next: OrderStatus) {
		if (!selectedOrder) return false;
		if (next === selectedOrder.status) return false;
		if (selectedOrder.status === 'new') {
			if (next === 'closed') return true;
			if (next === 'prepared') return allLinesReadyForPrepare;
			return false;
		}
		if (selectedOrder.status === 'prepared') {
			return next === 'new' || next === 'closed';
		}
		if (selectedOrder.status === 'invoiced') {
			return next === 'notified' || next === 'closed';
		}
		if (selectedOrder.status === 'notified') {
			return next === 'invoiced' || next === 'paid' || next === 'closed';
		}
		if (selectedOrder.status === 'paid') {
			return next === 'closed' || next === 'notified';
		}
		if (selectedOrder.status === 'closed') {
			return false;
		}
		return false;
	}

	async function editPreparedOrder() {
		if (!selectedOrder || selectedOrder.status !== 'prepared' || actionsLocked) return;
		closeStatusMenu();
		await setStatus('new');
	}

	function requestMarkPrepared() {
		if (!selectedOrder || !allLinesReadyForPrepare || actionsLocked) return;
		closeStatusMenu();
		prepareConfirmOpen = true;
	}

	function closePrepareConfirm() {
		prepareConfirmOpen = false;
	}

	async function confirmMarkPrepared() {
		if (!selectedOrder || !allLinesReadyForPrepare || actionsLocked) return;
		prepareConfirmOpen = false;
		await setStatus('prepared');
	}

	function requestSetAsInvoiced() {
		if (!selectedOrder || selectedOrder.status !== 'notified' || actionsLocked) return;
		closeStatusMenu();
		invoicedConfirmOpen = true;
	}

	function closeInvoicedConfirm() {
		invoicedConfirmOpen = false;
	}

	async function confirmSetAsInvoiced() {
		if (!selectedOrder || selectedOrder.status !== 'notified' || actionsLocked) return;
		invoicedConfirmOpen = false;
		await setStatus('invoiced');
	}

	async function markOrderPickedUp() {
		if (!selectedOrder || selectedOrder.status !== 'paid' || actionsLocked) return;
		closePaidMenu();
		await setStatus('closed', { closedReason: 'picked_up' });
	}

	/* async function unsetPaymentType() {
		if (!selectedOrder || selectedOrder.status !== 'paid' || actionsLocked) return;
		if (selectedOrder.paidReason === 'shopify') return;
		closePaidMenu();
		await setStatus('notified');
	} */

	/* async function reopenClosedOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (!canReopenClosed) return;
		const prev = statusBeforeClose(selectedOrder);
		if (!prev) return;
		closeStatusMenu();
		closePaidMenu();
		if (prev === 'paid') {
			await setStatus('paid', {
				paidReason: selectedOrder.paidReason ?? 'cash'
			});
			return;
		}
		if (prev === 'notified') {
			const channel: NotificationChannel =
				selectedOrder.notificationChannel ??
				(selectedOrder.merchTableOrder
					? 'in_person'
					: selectedOrder.discordHandle.trim()
						? 'discord'
						: 'email');
			await setStatus('notified', { notificationChannel: channel });
			return;
		}
		await setStatus(prev);
	}
  */

	function discordNotifyMessage(order: Order) {
		const handle = order.discordHandle.trim().replace(/^@/, '');
		return `@${handle} Artist Colony Invoice #${order.orderId} has been emailed to you. Please pay ASAP.`;
	}

	async function copyDiscordNotifyMessage() {
		if (!selectedOrder?.discordHandle.trim()) return;
		const text = discordNotifyMessage(selectedOrder);
		try {
			await navigator.clipboard.writeText(text);
			copyFlash = 'Copied';
			window.setTimeout(() => {
				if (copyFlash === 'Copied') copyFlash = '';
			}, 1600);
		} catch {
			error = 'Could not copy to clipboard.';
		}
	}

	async function markNotified(channel: 'discord' | 'email' | 'in_person') {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status !== 'invoiced') return;
		closeStatusMenu();
		discordNotifyOpen = false;
		if (!(await flushNotesIfNeeded({ quiet: true }))) return;
		saving = true;
		error = '';
		try {
			selectedOrder = await staffApi.markNotified(
				selectedOrder.orderId,
				channel,
				staffMeta()
			);
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to mark notified.';
		} finally {
			saving = false;
		}
	}

	function openDiscordNotify() {
		if (!selectedOrder || actionsLocked) return;
		if (!selectedOrder.discordHandle.trim()) return;
		copyFlash = '';
		discordNotifyOpen = true;
	}

	function closeDiscordNotify() {
		discordNotifyOpen = false;
		copyFlash = '';
	}

	async function sendInvoice() {
		if (!selectedOrder || selectedOrder.status !== 'prepared' || actionsLocked) return;
		if (!(await flushNotesIfNeeded({ quiet: true }))) return;
		closeStatusMenu();
		sendingInvoice = true;
		error = '';
		invoiceSuccess = '';
		openLineMenuIndex = null;
		try {
			const result = await staffApi.sendInvoice(selectedOrder.orderId, staffMeta());
			selectedOrder = result.order;
			invoiceSuccess = result.message;
			if (selectedOrder.merchTableOrder && selectedOrder.status === 'invoiced') {
				selectedOrder = await staffApi.markNotified(
					selectedOrder.orderId,
					'in_person',
					staffMeta()
				);
				const notifiedNote = 'Marked Notified in Person.';
				invoiceSuccess = /marked notified/i.test(result.message)
					? result.message
					: `${result.message} ${notifiedNote}`;
			}
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to send invoice.';
		} finally {
			sendingInvoice = false;
		}
	}

	async function setLineCollected(lineIndex: number, collected: boolean) {
		if (!selectedOrder || actionsLocked || !lineItemsEditable) return;
		saving = true;
		error = '';
		invoiceSuccess = '';
		try {
			selectedOrder = await staffApi.updateOrderLine(
				selectedOrder.orderId,
				lineIndex,
				{
					collected
				},
				staffMeta()
			);
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update collected state.';
		} finally {
			saving = false;
		}
	}

	async function setLineAction(lineIndex: number, lineAction: OrderLineAction) {
		if (!selectedOrder || actionsLocked || !lineItemsEditable) return;
		openLineMenuIndex = null;
		saving = true;
		error = '';
		invoiceSuccess = '';
		try {
			selectedOrder = await staffApi.updateOrderLine(
				selectedOrder.orderId,
				lineIndex,
				{
					lineAction,
					...(lineAction === 'sold_out' ? { collected: false } : {})
				},
				staffMeta()
			);
			await refreshOrdersList();
			if (lineAction === 'partial') {
				const qty = selectedOrder.items[lineIndex]?.collectedQuantity;
				partialQtyEditingIndex = lineIndex;
				partialQtyDraft = qty != null ? String(qty) : '';
				await tick();
				partialQtyInputEl?.focus();
				partialQtyInputEl?.select();
			} else if (partialQtyEditingIndex === lineIndex) {
				partialQtyEditingIndex = null;
				partialQtyDraft = '';
			}
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update line action.';
		} finally {
			saving = false;
		}
	}

	function toggleLineMenu(lineIndex: number) {
		if (actionsLocked || !lineItemsEditable) return;
		openLineMenuIndex = openLineMenuIndex === lineIndex ? null : lineIndex;
	}

	function closeLineMenu() {
		openLineMenuIndex = null;
	}

	function toggleLineCollectedFromItem(lineIndex: number, item: OrderItem) {
		if (!lineItemsEditable || actionsLocked || item.lineAction === 'sold_out') return;
		void setLineCollected(lineIndex, !item.collected);
	}

	function startPartialQtyEdit(lineIndex: number, item: OrderItem) {
		if (!lineItemsEditable || actionsLocked || item.lineAction !== 'partial') return;
		partialQtyEditingIndex = lineIndex;
		partialQtyDraft = item.collectedQuantity != null ? String(item.collectedQuantity) : '';
		void tick().then(() => {
			partialQtyInputEl?.focus();
			partialQtyInputEl?.select();
		});
	}

	function chooseLineAction(lineIndex: number, lineAction: OrderLineAction, current: OrderLineAction) {
		void setLineAction(lineIndex, current === lineAction ? '' : lineAction);
	}

	async function setCollectedQuantity(lineIndex: number, raw: string) {
		if (!selectedOrder || actionsLocked || !lineItemsEditable) return;
		const item = selectedOrder.items[lineIndex];
		if (!item) return;
		const parsed = Number.parseInt(raw, 10);
		const collectedQuantity = Number.isFinite(parsed)
			? Math.max(0, Math.min(item.quantity, parsed))
			: 0;
		saving = true;
		error = '';
		invoiceSuccess = '';
		try {
			selectedOrder = await staffApi.updateOrderLine(
				selectedOrder.orderId,
				lineIndex,
				{
					lineAction: 'partial',
					collectedQuantity
				},
				staffMeta()
			);
			await refreshOrdersList();
			partialQtyEditingIndex = null;
			partialQtyDraft = '';
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update collected quantity.';
		} finally {
			saving = false;
		}
	}

	async function savePartialQty(lineIndex: number) {
		await setCollectedQuantity(lineIndex, partialQtyDraft);
	}

	function qtyDisplay(item: OrderItem) {
		const effective = effectiveOrderItemQuantity(item);
		const adjusted = item.lineAction === 'partial' || item.lineAction === 'sold_out';
		return { effective, adjusted, original: item.quantity };
	}

	async function refresh() {
		if (tab === 'orders') await loadOrders();
		else await loadInventory();
	}

	onMount(() => {
		authed = Boolean(getStaffToken());
		authChecked = true;
		if (authed) {
			enableStaffSession();
			const stored = readStoredFilterState();
			statusChecks = stored.statusChecks;
			orderSort = stored.orderSort;
			orderQuery = stored.orderQuery;
			inventoryQuery = stored.inventoryQuery;
			const orderFromUrl = get(page).url.searchParams.get('order')?.trim() ?? '';
			void (async () => {
				await loadOrders();
				if (orderFromUrl) {
					await selectOrder(orderFromUrl);
				}
			})();
		}
	});

	async function handleLogin(event: SubmitEvent) {
		event.preventDefault();
		if (loginBusy) return;
		loginBusy = true;
		loginError = '';
		try {
			await loginStaff(loginUsername, loginPassword);
			authed = true;
			loginPassword = '';
			const stored = readStoredFilterState();
			statusChecks = stored.statusChecks;
			orderSort = stored.orderSort;
			orderQuery = stored.orderQuery;
			inventoryQuery = stored.inventoryQuery;
			await loadOrders();
		} catch (err) {
			loginError = err instanceof Error ? err.message : 'Could not sign in.';
		} finally {
			loginBusy = false;
		}
	}

	async function switchTab(next: Tab) {
		tab = next;
		menuOpen = false;
		statusFilterOpen = false;
		searchOpen = false;
		soldOutMenuItemId = '';
		if (next === 'inventory') await clearSelectedOrder();
		void refresh();
	}

	function toggleMenu() {
		menuOpen = !menuOpen;
	}

	function closeMenu() {
		menuOpen = false;
	}

	function closeStatusFilter() {
		statusFilterOpen = false;
	}

	function closeSearch() {
		searchOpen = false;
	}

	async function clearSearch() {
		const hadQuery =
			tab === 'orders' ? orderQuery.trim().length > 0 : inventoryQuery.trim().length > 0;
		if (tab === 'orders') orderQuery = '';
		else inventoryQuery = '';
		if (hadQuery) {
			if (tab === 'orders') await loadOrders();
			else await loadInventory();
		}
		await tick();
		searchInputEl?.focus();
	}

	function toggleStatusFilter() {
		statusFilterOpen = !statusFilterOpen;
		if (statusFilterOpen) searchOpen = false;
	}

	async function toggleSearch() {
		searchOpen = !searchOpen;
		if (searchOpen) {
			statusFilterOpen = false;
			await tick();
			searchInputEl?.focus();
			return;
		}
		const hadQuery =
			tab === 'orders' ? orderQuery.trim().length > 0 : inventoryQuery.trim().length > 0;
		if (tab === 'orders') orderQuery = '';
		else inventoryQuery = '';
		if (hadQuery) {
			if (tab === 'orders') await loadOrders();
			else await loadInventory();
		}
	}

	async function logOut() {
		await logoutStaff();
		menuOpen = false;
		authed = false;
		selectedOrderId = '';
		selectedOrder = null;
		orders = [];
		inventory = [];
	}

	function formatWhen(iso: string) {
		try {
			return new Date(iso).toLocaleString(undefined, {
				month: 'short',
				day: 'numeric',
				hour: 'numeric',
				minute: '2-digit'
			});
		} catch {
			return iso;
		}
	}

	function orderIdBase(orderId: string, merchTableOrder?: boolean) {
		return merchTableOrder && orderId.endsWith('M') ? orderId.slice(0, -1) : orderId;
	}

	function orderIdHasMerchSuffix(orderId: string, merchTableOrder?: boolean) {
		return Boolean(merchTableOrder && orderId.endsWith('M'));
	}

	async function toggleSoldOut(item: InventoryItem) {
		if (savingSoldOutId) return;
		savingSoldOutId = item.id;
		error = '';
		soldOutMenuItemId = '';
		try {
			const updated = await staffApi.setProductSoldOut(item.id, !item.soldOut);
			inventory = inventory.map((entry) => (entry.id === updated.id ? updated : entry));
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update sold-out state.';
		} finally {
			savingSoldOutId = '';
		}
	}

	function toggleSoldOutMenu(itemId: string) {
		if (savingSoldOutId) return;
		soldOutMenuItemId = soldOutMenuItemId === itemId ? '' : itemId;
	}

	function closeSoldOutMenu() {
		soldOutMenuItemId = '';
	}

	function isMerchTableInventory(item: InventoryItem) {
		return (item.productDisplay ?? '').trim().toLowerCase() === 'merch table';
	}

	$: inventoryGroups = (() => {
		const groups: { shop: string; items: InventoryItem[] }[] = [];
		for (const item of inventory) {
			const shop = (item.shopName || '').trim() || '—';
			const last = groups[groups.length - 1];
			if (!last || last.shop !== shop) {
				groups.push({ shop, items: [item] });
			} else {
				last.items.push(item);
			}
		}
		return groups;
	})();
</script>

<svelte:head>
	<title>Staff Management · Artist Colony</title>
</svelte:head>

<svelte:window
	on:keydown={(event) => {
		if (event.key === 'Escape') {
			closeMenu();
			closeLineMenu();
			closePaidMenu();
			closeStatusMenu();
			closeStatusFilter();
			closeSearch();
		}
	}}
/>

<div class="staff-page" class:staff-page-login={!authed}>
	{#if !authed}
		<header class="staff-login-header">
			<h1>Staff Sign In</h1>
		</header>
		<div class="staff-login-body">
			<section class="staff-login-panel">
				{#if authChecked}
					<form class="staff-login-form" on:submit={handleLogin}>
						<label>
							Username
							<input
								type="text"
								autocomplete="username"
								bind:value={loginUsername}
								disabled={loginBusy}
							/>
						</label>
						<label>
							Password
							<input
								type="password"
								autocomplete="current-password"
								bind:value={loginPassword}
								disabled={loginBusy}
							/>
						</label>
						{#if loginError}
							<p class="staff-login-error">{loginError}</p>
						{/if}
						<button
							type="submit"
							class="add-to-cart-button staff-login-submit"
							disabled={loginBusy || !loginUsername.trim() || !loginPassword}
						>
							{loginBusy ? 'Signing in…' : 'Sign In'}
						</button>
					</form>
				{:else}
					<p class="staff-muted">Loading…</p>
				{/if}
			</section>
		</div>
	{:else}
	<header class="staff-bar">
		<div class="staff-bar-filters" class:staff-bar-filters-hidden={hideBarFilters}>
			{#if tab === 'orders'}
				<div class="staff-status-filter" class:open={statusFilterOpen}>
					<button
						type="button"
						class="staff-status-filter-trigger"
						aria-expanded={statusFilterOpen}
						aria-haspopup="listbox"
						aria-label="Filter by status"
						on:click={toggleStatusFilter}
					>
						<span class="staff-status-filter-main">
							<span class="staff-status-filter-label">{statusFilterLabel}</span>
							{#if filteredStatuses.length > 0}
								<span class="staff-status-filter-dots" aria-hidden="true">
									{#each filteredStatuses as status}
										<span class="staff-status-filter-dot" data-status={status}></span>
									{/each}
								</span>
							{/if}
						</span>
						<span class="staff-status-filter-caret" aria-hidden="true"></span>
					</button>
					{#if statusFilterOpen}
						<button
							type="button"
							class="staff-status-filter-backdrop"
							tabindex="-1"
							aria-label="Close status filters"
							on:click={closeStatusFilter}
						></button>
					{/if}
					<div
						class="staff-status-checks"
						role="group"
						aria-label="Filter by status"
						on:click|stopPropagation
					>
						<button
							type="button"
							class="staff-status-show-all"
							on:click|stopPropagation={showAllStatuses}
						>
							Select All
						</button>
						{#each ORDER_STATUSES as status}
							<label class="staff-status-check" data-status={status}>
								<input
									type="checkbox"
									bind:checked={statusChecks[status]}
									on:change={onStatusFilterChange}
								/>
								<span>{ORDER_STATUS_LABELS[status]}</span>
							</label>
						{/each}
					</div>
				</div>
				<div
					class="staff-search-wrap"
					class:open={searchOpen}
					class:has-query={orderQuery.trim().length > 0}
				>
					<button
						type="button"
						class="staff-search-toggle"
						aria-expanded={searchOpen}
						aria-label={searchOpen ? 'Hide search' : 'Show search'}
						on:click={toggleSearch}
					>
						<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none">
							<circle cx="11" cy="11" r="6.5" stroke="currentColor" stroke-width="2" />
							<path
								d="M16.5 16.5L20 20"
								stroke="currentColor"
								stroke-width="2"
								stroke-linecap="round"
							/>
						</svg>
					</button>
					<div class="staff-search-field">
						<input
							class="staff-bar-search"
							type="search"
							aria-label="Search"
							placeholder="Search"
							bind:this={searchInputEl}
							bind:value={orderQuery}
							on:keydown={(event) => event.key === 'Enter' && loadOrders()}
						/>
						{#if orderQuery.trim()}
							<button
								type="button"
								class="staff-search-clear"
								aria-label="Clear search"
								on:click={clearSearch}
							>
								×
							</button>
						{/if}
					</div>
				</div>
				<button
					type="button"
					class="staff-sort-btn"
					aria-label={
						orderSort === 'oldest'
							? 'Sorted oldest to newest. Switch to newest first.'
							: 'Sorted newest to oldest. Switch to oldest first.'
					}
					title={orderSort === 'oldest' ? 'Oldest First' : 'Newest First'}
					on:click={toggleOrderSort}
				>
					<span
						class="staff-sort-icon"
						class:newest={orderSort === 'newest'}
						aria-hidden="true"
					>
						<svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="square">
							<path d="M8 6v12" />
							<path d="M8 18l-3-3" />
							<path d="M8 18l3-3" />
							<path d="M14 7h6" />
							<path d="M14 12h4" />
							<path d="M14 17h2" />
						</svg>
					</span>
					<span class="staff-sort-label">
						{orderSort === 'oldest' ? 'Oldest First' : 'Newest First'}
					</span>
				</button>
			{:else}
				<div
					class="staff-search-wrap staff-search-wrap-inventory"
					class:has-query={inventoryQuery.trim().length > 0}
				>
					<div class="staff-search-field">
						<input
							class="staff-bar-search"
							type="search"
							aria-label="Search"
							placeholder="Search"
							bind:this={searchInputEl}
							bind:value={inventoryQuery}
							on:keydown={(event) => event.key === 'Enter' && loadInventory()}
						/>
						{#if inventoryQuery.trim()}
							<button
								type="button"
								class="staff-search-clear"
								aria-label="Clear search"
								on:click={clearSearch}
							>
								×
							</button>
						{/if}
					</div>
				</div>
			{/if}
		</div>

		{#if tab === 'orders' && selectedOrder}
			<button type="button" class="staff-bar-back" on:click={() => clearSelectedOrder()}>
				← All orders
			</button>
		{/if}

		<button
			type="button"
			class="staff-menu-btn"
			aria-expanded={menuOpen}
			aria-controls="staff-menu"
			aria-label={menuOpen ? 'Close menu' : 'Open menu'}
			on:click={toggleMenu}
		>
			<span class="staff-menu-icon" class:open={menuOpen} aria-hidden="true"></span>
		</button>

		{#if menuOpen}
			<nav id="staff-menu" class="staff-menu" aria-label="Staff sections">
				<button
					type="button"
					class:active={tab === 'orders'}
					on:click={() => switchTab('orders')}
				>
					Orders
				</button>
				<button
					type="button"
					class:active={tab === 'inventory'}
					on:click={() => switchTab('inventory')}
				>
					Inventory
				</button>
				<a href="{base}/" on:click={closeMenu}>Price List</a>
				<button type="button" on:click={logOut}>Log Out</button>
			</nav>
		{/if}
	</header>

	{#if menuOpen}
		<button type="button" class="staff-menu-backdrop" aria-label="Close menu" on:click={closeMenu}
		></button>
	{/if}

	{#if error}
		<p class="staff-error" role="alert">{error}</p>
	{/if}

	{#if tab === 'orders'}
		<div class="staff-split" class:staff-split-detail={!!selectedOrder}>
			<div class="staff-panel staff-panel-list">
				{#if loading && !orders.length}
					<p class="staff-muted">Loading orders…</p>
				{:else if !orders.length}
					<p class="staff-muted">No orders match.</p>
				{:else}
					<ul class="staff-card-list">
						{#each orders as order}
							<li>
								<button
									type="button"
									class="staff-card"
									class:selected={order.orderId === selectedOrderId}
									on:click={() => selectOrder(order.orderId)}
								>
									<div class="staff-card-top">
										<span class="staff-strong">
											{orderIdBase(order.orderId, order.merchTableOrder)}{#if orderIdHasMerchSuffix(order.orderId, order.merchTableOrder)}<span
													class="staff-merch-m">M</span
												>{/if}
										</span>
										<span
											class="staff-status"
											data-status={orderDisplayStatusKey(order)}
										>
											{#if order.status === 'paid'}
												<span class="staff-status-paid-label">
													Paid<span
														class="staff-paid-check"
														title={order.paidReason
															? PAID_REASON_LABELS[order.paidReason]
															: 'Paid'}
														aria-hidden="true"
													>
														<svg
															viewBox="0 0 24 24"
															width="12"
															height="12"
															aria-hidden="true"
															fill="none"
														>
															<path
																d="M5 13l4 4L19 7"
																stroke="currentColor"
																stroke-width="2.5"
																stroke-linecap="round"
																stroke-linejoin="round"
															/>
														</svg>
													</span>
												</span>
											{:else}
												{orderDisplayLabel(order)}
											{/if}
										</span>
									</div>
									<div class="staff-card-main">
										<span class="staff-strong">
											{order.name}{#if order.submittedByStaffName?.trim()}
												<span class="staff-submitted-by"
													>(Submitted by {order.submittedByStaffName.trim()})</span
												>
											{/if}
										</span>
										<span class="staff-card-total">{formatCents(order.subtotalCents)}</span>
									</div>
									<div class="staff-muted">
										{order.email}{#if order.discordHandle.trim()}
											· {order.discordHandle}{/if} · {formatWhen(order.submittedAt)}
									</div>
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			<div class="staff-panel staff-panel-detail">
				{#if !selectedOrder}
					<p class="staff-muted staff-detail-empty">Select an order to view details.</p>
				{:else}
					<div class="staff-detail-head">
						<div class="staff-detail-status">
							{#if selectedOrder.status === 'new' || selectedOrder.status === 'prepared'}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status={selectedOrder.status}
										class:open={statusMenuOpen}
										disabled={actionsLocked || sendingInvoice}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span
											>{selectedOrder.status === 'new' ? 'New' : 'Prepared'}</span
										>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if statusMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close status menu"
											on:click={closeStatusMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											{#if selectedOrder.status === 'new'}
												<button
													type="button"
													role="menuitem"
													disabled={actionsLocked || !allLinesReadyForPrepare}
													title={allLinesReadyForPrepare
														? undefined
														: 'Check off every line (or mark sold out) before preparing'}
													on:click={requestMarkPrepared}
												>
													Prepared
												</button>
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-cancel staff-status-menu-divider"
													disabled={actionsLocked || sendingInvoice}
													on:click={requestCancelOrder}
												>
													Close Order
												</button>
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-duplicate"
													disabled={actionsLocked}
													on:click={duplicateOrder}
												>
													Duplicate Order
												</button>
											{:else}
												<button
													type="button"
													role="menuitem"
													disabled={actionsLocked || sendingInvoice}
													on:click={sendInvoiceFromMenu}
												>
													{sendingInvoice ? 'Sending Invoice…' : 'Send Invoice'}
												</button>
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-back"
													disabled={actionsLocked || sendingInvoice}
													on:click={editPreparedOrder}
												>
													Edit Order
												</button>
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-cancel staff-status-menu-divider"
													disabled={actionsLocked || sendingInvoice}
													on:click={requestCancelOrder}
												>
													Close Order
												</button>
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-duplicate"
													disabled={actionsLocked}
													on:click={duplicateOrder}
												>
													Duplicate Order
												</button>
											{/if}
										</div>
									{/if}
								</div>
							{/if}
							{#if selectedOrder.status === 'invoiced'}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="invoiced"
										class:open={statusMenuOpen}
										disabled={actionsLocked || sendingInvoice}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>Invoiced</span>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if statusMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close status menu"
											on:click={closeStatusMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											{#if selectedOrder.merchTableOrder}
												<button
													type="button"
													role="menuitem"
													disabled={actionsLocked}
													on:click={() => markNotified('in_person')}
												>
													Notified In Person
												</button>
											{:else}
												<button
													type="button"
													role="menuitem"
													disabled={actionsLocked}
													on:click={() => {
														if (selectedOrder?.discordHandle.trim()) {
															closeStatusMenu();
															openDiscordNotify();
														} else {
															void markNotified('email');
														}
													}}
												>
													{selectedOrder.discordHandle.trim()
														? 'Notify on Discord'
														: 'Mark as Notified'}
												</button>
											{/if}
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-email"
												disabled={actionsLocked || sendingInvoice}
												on:click={resendInvoiceFromMenu}
											>
												{sendingInvoice ? 'Resending Invoice…' : 'Resend Invoice'}
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked || sendingInvoice}
												on:click={requestCancelOrder}
											>
												Close Order
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-duplicate"
												disabled={actionsLocked}
												on:click={duplicateOrder}
											>
												Duplicate Order
											</button>
										</div>
									{/if}
								</div>
							{/if}
							{#if selectedOrder.status === 'notified'}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="notified"
										class:open={statusMenuOpen}
										disabled={actionsLocked || sendingInvoice}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>Notified</span>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if statusMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close status menu"
											on:click={closeStatusMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											<button
												type="button"
												role="menuitem"
												class="staff-paid-menu-shopify"
												disabled
												title="Only Shopify can set this"
											>
												{PAY_ACTION_LABELS.shopify}
											</button>
											{#each MANUAL_PAID_REASONS as reason}
												<button
													type="button"
													role="menuitem"
													disabled={actionsLocked}
													on:click={() => choosePaidReason(reason)}
												>
													{PAY_ACTION_LABELS[reason]}
												</button>
											{/each}
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-back"
												disabled={actionsLocked}
												on:click={requestSetAsInvoiced}
											>
												Set As Invoiced
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-email"
												disabled={actionsLocked || sendingInvoice}
												on:click={resendInvoiceFromMenu}
											>
												{sendingInvoice ? 'Resending Invoice…' : 'Resend Invoice'}
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked}
												on:click={requestCancelOrder}
											>
												Close Order
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-duplicate"
												disabled={actionsLocked}
												on:click={duplicateOrder}
											>
												Duplicate Order
											</button>
										</div>
									{/if}
								</div>
							{/if}
							{#if selectedOrder.status === 'paid'}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="paid"
										class:open={paidMenuOpen}
										disabled={actionsLocked}
										aria-expanded={paidMenuOpen}
										aria-haspopup="menu"
										on:click={togglePaidMenu}
									>
										<span class="staff-paid-trigger-label">
											{paidMethodLabel}<span
												class="staff-paid-check"
												title={paidMethodLabel}
												aria-hidden="true"
											>
												<svg
													viewBox="0 0 24 24"
													width="14"
													height="14"
													aria-hidden="true"
													fill="none"
												>
													<path
														d="M5 13l4 4L19 7"
														stroke="currentColor"
														stroke-width="2.5"
														stroke-linecap="round"
														stroke-linejoin="round"
													/>
												</svg>
											</span>
										</span>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if paidMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close paid menu"
											on:click={closePaidMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											<button
												type="button"
												role="menuitem"
												disabled={actionsLocked}
												on:click={markOrderPickedUp}
											>
												Order Picked Up
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked}
												on:click={requestCancelOrder}
											>
												Close Order
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-duplicate"
												disabled={actionsLocked}
												on:click={duplicateOrder}
											>
												Duplicate Order
											</button>
										</div>
									{/if}
								</div>
							{/if}
							{#if isPickedUp}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="picked_up"
										class:open={statusMenuOpen}
										disabled={actionsLocked}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>Picked Up</span>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if statusMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close status menu"
											on:click={closeStatusMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-duplicate"
												disabled={actionsLocked}
												on:click={duplicateOrder}
											>
												Duplicate Order
											</button>
										</div>
									{/if}
								</div>
							{:else if selectedOrder.status === 'closed'}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="closed"
										class:open={statusMenuOpen}
										disabled={actionsLocked}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>{orderDisplayLabel(selectedOrder)}</span>
										<span class="staff-paid-caret" aria-hidden="true"></span>
									</button>
									{#if statusMenuOpen}
										<button
											type="button"
											class="staff-paid-menu-backdrop"
											aria-label="Close status menu"
											on:click={closeStatusMenu}
										></button>
										<div class="staff-paid-menu" role="menu">
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-duplicate"
												disabled={actionsLocked}
												on:click={duplicateOrder}
											>
												Duplicate Order
											</button>
										</div>
									{/if}
								</div>
							{/if}
						</div>
						<div class="staff-detail-main">
							<div class="staff-order-ids">
								<h2>
									{orderIdBase(selectedOrder.orderId, selectedOrder.merchTableOrder)}{#if orderIdHasMerchSuffix(selectedOrder.orderId, selectedOrder.merchTableOrder)}<span
											class="staff-merch-m">M</span
										>{/if}
								</h2>
								{#if selectedOrder.shopifyInvoiceId}
									{#if shopifyOrderLink || shopifyDraftLink}
										<a
											class="staff-shopify-order"
											class:paid={selectedOrder.status === 'paid' ||
												(selectedOrder.status === 'closed' &&
													selectedOrder.closedReason === 'picked_up')}
											href={shopifyOrderLink ?? shopifyDraftLink}
											target="_blank"
											rel="noreferrer"
											title={shopifyOrderLink
												? 'Open Shopify Order'
												: 'Open Shopify Draft Order'}
										>
											{selectedOrder.shopifyInvoiceId}
										</a>
									{:else}
										<span
											class="staff-shopify-order"
											class:paid={selectedOrder.status === 'paid' ||
												(selectedOrder.status === 'closed' &&
													selectedOrder.closedReason === 'picked_up')}
											title="Shopify order"
										>
											{selectedOrder.shopifyInvoiceId}
										</span>
									{/if}
								{/if}
							</div>
							<p class="staff-muted">
								{selectedOrder.name}{#if selectedOrder.submittedByStaffName?.trim()}
									<span class="staff-submitted-by"
										>(Submitted by {selectedOrder.submittedByStaffName.trim()})</span
									>
								{/if}
								{#if selectedOrder.discordHandle}
									· {selectedOrder.discordHandle}
								{/if}
							</p>
							<p class="staff-muted">{selectedOrder.email}</p>
						</div>
					</div>

					{#if selectedOrder.status === 'invoiced' && !selectedOrder.merchTableOrder}
						<div class="staff-detail-pre-actions">
							{#if invoiceSuccess}
								<p class="staff-invoice-success" role="status">{invoiceSuccess}</p>
							{/if}
							{#if selectedOrder.discordHandle.trim()}
								<button
									type="button"
									class="staff-confirm-btn staff-detail-action-btn"
									data-status="notified"
									disabled={actionsLocked}
									on:click={openDiscordNotify}
								>
									Notify on Discord
								</button>
							{:else}
								<p class="staff-notify-notice" role="status">
									This customer did not provide a Discord handle. After confirming they received
									the invoice email, Mark as Notified.
								</p>
							{/if}
						</div>
					{:else if invoiceSuccess}
						<div class="staff-detail-pre-actions">
							<p class="staff-invoice-success" role="status">{invoiceSuccess}</p>
						</div>
					{/if}

					{#if selectedOrder.status === 'prepared'}
						<div class="staff-detail-pre-actions staff-detail-pre-actions-spaced">
							<button
								type="button"
								class="staff-confirm-btn staff-detail-action-btn"
								data-status="invoiced"
								disabled={actionsLocked || sendingInvoice}
								on:click={sendInvoiceFromMenu}
							>
								{sendingInvoice ? 'Sending Invoice…' : 'Send Invoice'}
							</button>
						</div>
					{:else if selectedOrder.status === 'paid'}
						<div class="staff-detail-pre-actions staff-detail-pre-actions-spaced">
							<button
								type="button"
								class="staff-confirm-btn staff-detail-action-btn"
								data-status="paid"
								disabled={actionsLocked}
								on:click={markOrderPickedUp}
							>
								Order Picked Up
							</button>
						</div>
					{:else if selectedOrder.status === 'notified'}
						<div class="staff-detail-pre-actions staff-detail-pre-actions-spaced">
							<button
								type="button"
								class="staff-confirm-btn staff-detail-action-btn staff-refresh-payment-btn"
								on:click={() => window.location.reload()}
							>
								<span>Refresh Order Payment Status</span>
								<svg
									class="staff-refresh-icon"
									viewBox="0 -3 24 27"
									width="20"
									height="20"
									aria-hidden="true"
									fill="none"
									overflow="visible"
								>
									<path
										d="M20.5 12a8.5 8.5 0 1 1-4.5-7.2"
										stroke="currentColor"
										stroke-width="2.25"
										stroke-linecap="round"
									/>
									<path
										d="M20.5 3.5v7.5H13"
										stroke="currentColor"
										stroke-width="2.25"
										stroke-linecap="round"
										stroke-linejoin="round"
										transform="translate(-2 -4.5) rotate(-20 20.5 11)"
									/>
								</svg>
							</button>
						</div>
					{/if}

					<div class="staff-line-section">
						<div
							class="staff-line-table"
							class:staff-line-table-readonly={lineItemsHardLocked}
							class:staff-line-table-softlocked={lineItemsSoftLocked}
							role="table"
							aria-label="Order items"
						>
							<ul class="staff-line-list">
								{#each selectedOrder.items as item, lineIndex}
									{@const qty = qtyDisplay(item)}
									{@const lineCheckable =
										lineItemsEditable && item.lineAction !== 'sold_out' && !actionsLocked}
									{@const partialQtyEditing =
										lineItemsEditable &&
										item.lineAction === 'partial' &&
										partialQtyEditingIndex === lineIndex}
									<li
										class="staff-line"
										class:staff-row-sold-out={item.lineAction === 'sold_out'}
										class:staff-row-collected={
											lineItemsEditable &&
											item.collected &&
											item.lineAction !== 'sold_out'
										}
										role="row"
									>
										{#if !lineItemsHardLocked}
											<label
												class="staff-line-col-collected"
												class:staff-line-checkable={lineCheckable}
											>
												{#if item.lineAction !== 'sold_out'}
													<input
														type="checkbox"
														checked={item.collected}
														disabled={actionsLocked || lineItemsSoftLocked}
														aria-label={`Collected ${item.title}`}
														on:change={(event) =>
															setLineCollected(lineIndex, event.currentTarget.checked)}
													/>
												{:else}
													<span class="staff-collected-na" aria-hidden="true">—</span>
												{/if}
											</label>
										{/if}

										<div
											class="staff-line-col-item staff-line-body"
											class:staff-line-checkable={lineCheckable}
											role={lineCheckable ? 'button' : undefined}
											tabindex={lineCheckable ? 0 : undefined}
											on:click={() => toggleLineCollectedFromItem(lineIndex, item)}
											on:keydown={(event) => {
												if (!lineCheckable) return;
												if (event.key === 'Enter' || event.key === ' ') {
													event.preventDefault();
													toggleLineCollectedFromItem(lineIndex, item);
												}
											}}
										>
											<div class="staff-strong">{item.title}</div>
											<div class="staff-muted">
												{item.productCode || 'No code'} · {item.artist}
											</div>
											{#if item.lineAction === 'sold_out'}
												<div class="staff-sold-out-label">Sold out</div>
											{:else if item.lineAction === 'partial'}
												<div class="staff-partial-label">
													{item.collected ? 'Partially Collected' : 'Quantity Updated'}
												</div>
											{:else if lineItemsHardLocked && item.collected}
												<div class="staff-partial-label">Collected</div>
											{/if}
										</div>

										<div
											class="staff-line-col-qty"
											class:staff-line-col-qty-editing={partialQtyEditing}
											aria-label={`Quantity ${qty.effective}`}
										>
											{#if item.lineAction === 'partial' && lineItemsEditable}
												<span class="staff-qty-original">{item.quantity}</span>
												{#if partialQtyEditing}
													<input
														bind:this={partialQtyInputEl}
														type="number"
														class="staff-partial-qty-input"
														min="0"
														max={item.quantity}
														inputmode="numeric"
														disabled={actionsLocked}
														bind:value={partialQtyDraft}
														aria-label={`Collected quantity for ${item.title}`}
														on:click|stopPropagation
														on:keydown={(event) => {
															if (event.key === 'Enter') {
																event.preventDefault();
																void savePartialQty(lineIndex);
															}
														}}
													/>
													<button
														type="button"
														class="staff-partial-qty-save"
														disabled={actionsLocked}
														on:click|stopPropagation={() => savePartialQty(lineIndex)}
													>
														Save
													</button>
												{:else}
													<button
														type="button"
														class="staff-qty-effective staff-qty-edit-btn"
														disabled={actionsLocked}
														aria-label={`Edit collected quantity for ${item.title}`}
														on:click|stopPropagation={() =>
															startPartialQtyEdit(lineIndex, item)}
													>
														{item.collectedQuantity ?? qty.effective}
													</button>
												{/if}
											{:else if qty.adjusted}
												<span class="staff-qty-original">{qty.original}</span>
												<span class="staff-qty-effective">{qty.effective}</span>
											{:else}
												<span>{qty.original}</span>
											{/if}
										</div>

										<div class="staff-line-col-price">
											<span class="staff-line-price">{formatCents(item.lineSubtotalCents)}</span>
										</div>

										{#if lineItemsEditable}
											<div class="staff-line-col-action staff-line-actions">
												<div
													class="staff-line-menu-wrap"
													class:open={openLineMenuIndex === lineIndex}
												>
													<button
														type="button"
														class="staff-line-caret"
														class:open={openLineMenuIndex === lineIndex}
														aria-expanded={openLineMenuIndex === lineIndex}
														aria-haspopup="menu"
														aria-label={`More actions for ${item.title}`}
														disabled={actionsLocked}
														on:click={() => toggleLineMenu(lineIndex)}
													>
														<span class="staff-line-caret-icon" aria-hidden="true"></span>
													</button>
													{#if openLineMenuIndex === lineIndex}
														<button
															type="button"
															class="staff-line-menu-backdrop"
															aria-label="Close line menu"
															on:click={closeLineMenu}
														></button>
														<div class="staff-line-menu" role="menu">
															<button
																type="button"
																role="menuitem"
																class:staff-line-menu-undo={item.lineAction === 'partial'}
																disabled={actionsLocked}
																on:click={() =>
																	chooseLineAction(lineIndex, 'partial', item.lineAction)}
															>
																{item.lineAction === 'partial'
																	? item.collected
																		? 'Undo Partially Collected'
																		: 'Undo Quantity Update'
																	: 'Partially Collected'}
															</button>
															<button
																type="button"
																role="menuitem"
																class:staff-line-menu-undo={item.lineAction === 'sold_out'}
																disabled={actionsLocked}
																on:click={() =>
																	chooseLineAction(lineIndex, 'sold_out', item.lineAction)}
															>
																{item.lineAction === 'sold_out'
																	? 'Undo Sold Out'
																	: 'Mark As Sold Out'}
															</button>
														</div>
													{/if}
												</div>
											</div>
										{/if}
									</li>
								{/each}
							</ul>
						</div>
					</div>

					<div
						class="staff-total"
						class:staff-total-readonly={lineItemsHardLocked}
						class:staff-total-softlocked={lineItemsSoftLocked}
						class:staff-total-editable={lineItemsEditable}
					>
						{#if !lineItemsHardLocked}
							<span class="staff-total-spacer staff-total-collected" aria-hidden="true"></span>
						{/if}
						<span class="staff-total-label">Total</span>
						<span class="staff-total-spacer staff-total-qty" aria-hidden="true"></span>
						<span class="staff-total-amount">{formatCents(selectedOrder.subtotalCents)}</span>
						{#if lineItemsEditable}
							<span class="staff-total-spacer staff-total-action" aria-hidden="true"></span>
						{/if}
					</div>

					{#if (selectedOrder.status === 'invoiced' && !selectedOrder.discordHandle.trim()) ||
						selectedOrder.status === 'new'}
						<div class="staff-detail-post-actions">
							{#if selectedOrder.status === 'invoiced' && !selectedOrder.discordHandle.trim()}
								<button
									type="button"
									class="staff-confirm-btn staff-detail-action-btn"
									data-status="notified"
									disabled={actionsLocked}
									on:click={() => markNotified('email')}
								>
									Mark as Notified
								</button>
							{/if}

							{#if selectedOrder.status === 'new'}
								<button
									type="button"
									class="staff-confirm-btn staff-detail-action-btn"
									data-status="prepared"
									disabled={actionsLocked || !canSelectStatus('prepared')}
									title={allLinesReadyForPrepare
										? undefined
										: 'Check off every line (or mark sold out) before preparing'}
									on:click={requestMarkPrepared}
								>
									Order is Prepared!
								</button>
							{/if}
						</div>
					{/if}

					<label class="staff-notes">
						Staff notes
						<textarea
							rows="3"
							bind:value={notesDraft}
							disabled={actionsLocked}
							on:blur={() => flushNotesIfNeeded({ quiet: true })}
						></textarea>
					</label>

					<section class="staff-history" aria-label="Status history">
						<h3 class="staff-history-title">Status history</h3>
						{#if statusHistoryEntries.length === 0}
							<p class="staff-muted">No status changes yet.</p>
						{:else}
							<ol class="staff-history-list">
								{#each statusHistoryEntries as entry}
									<li class="staff-history-item">
										<div class="staff-history-summary">{entry.summary}</div>
										<div class="staff-history-meta">
											<span class="staff-history-staff">{entry.staffName}</span>
											<span class="staff-history-when">{formatWhen(entry.at)}</span>
										</div>
									</li>
								{/each}
							</ol>
						{/if}
					</section>
				{/if}
			</div>
		</div>
	{:else}
		<section class="staff-panel staff-inventory-panel">
			{#if loading && !inventory.length}
				<p class="staff-muted staff-inventory-status">Loading inventory…</p>
			{:else if !inventory.length}
				<p class="staff-muted staff-inventory-status">No inventory items match.</p>
			{:else}
				<div class="staff-inventory">
					<table class="staff-inventory-table min-w-full">
						<colgroup>
							<col class="col-artist" />
							<col class="col-code" />
							<col class="col-product" />
							<col class="col-type" />
							<col class="col-cad" />
							<col class="col-cart" />
						</colgroup>
						<thead>
							<tr class="col-sizer">
								<th class="artist-col"></th>
								<th class="product-code-col"></th>
								<th class="product-col"></th>
								<th class="type-col"></th>
								<th class="cad-col"></th>
								<th class="cart-col"></th>
							</tr>
						</thead>
						{#each inventoryGroups as group}
							<tbody>
								<tr class="artist-divider">
									<td colspan="6">
										<ArtistGroupHeading artist={group.shop} />
									</td>
								</tr>
								{#each group.items as item, i}
									<tr class:stripe-alt={i % 2 === 1} class:staff-inv-sold-out={item.soldOut}>
										<td>
											<span class:small-text={(item.artistName || '').length > 15}
												>{item.artistName || '—'}</span
											>
										</td>
										<td class="font-bold">
											{#if isMerchTableInventory(item)}
												{#if (item.productCode ?? '').trim()}
													<div class="merch-table-code">{item.productCode}</div>
												{/if}
												<em class="unique-item-note">*Merch Table</em>
											{:else}
												{item.productCode || '—'}
											{/if}
										</td>
										<td>
											<div class="font-bold">{item.title}</div>
										</td>
										<td class="type-col">
											{#if item.itemType}
												<div class="text-sm text-gray-600">{item.itemType}</div>
											{/if}
										</td>
										<td class="cad-col font-bold">
											<div class="price-cad">{formatCents(item.priceCents)}</div>
										</td>
										<td class="cart-cell">
											<div
												class="staff-inv-sold-menu-wrap"
												class:open={soldOutMenuItemId === item.id}
											>
												<button
													type="button"
													class="staff-inv-sold-trigger"
													class:open={soldOutMenuItemId === item.id}
													class:is-sold-out={item.soldOut}
													disabled={savingSoldOutId === item.id}
													aria-expanded={soldOutMenuItemId === item.id}
													aria-haspopup="menu"
													on:click={() => toggleSoldOutMenu(item.id)}
												>
													<span class="staff-inv-sold-label">
														{#if savingSoldOutId === item.id}
															Saving…
														{:else if item.soldOut}
															Sold Out
														{:else}
															<span class="staff-inv-avail-full">Available</span>
															<span class="staff-inv-avail-short">Avail.</span>
														{/if}
													</span>
													<span class="staff-paid-caret" aria-hidden="true"></span>
												</button>
												{#if soldOutMenuItemId === item.id}
													<button
														type="button"
														class="staff-paid-menu-backdrop"
														aria-label="Close availability menu"
														on:click={closeSoldOutMenu}
													></button>
													<div class="staff-paid-menu staff-inv-sold-menu" role="menu">
														<button
															type="button"
															role="menuitem"
															disabled={savingSoldOutId === item.id}
															on:click={() => toggleSoldOut(item)}
														>
															{item.soldOut ? 'Mark as Available' : 'Mark as Sold Out'}
														</button>
													</div>
												{/if}
											</div>
										</td>
									</tr>
								{/each}
							</tbody>
						{/each}
					</table>

					<div class="staff-inventory-mobile mobile-items">
						{#each inventoryGroups as group}
							<section class="mobile-artist-group">
								<div class="mobile-artist-divider">
									<ArtistGroupHeading artist={group.shop} />
								</div>
								{#each group.items as item, i}
									<div
										class="mobile-item"
										class:stripe-alt={i % 2 === 1}
										class:staff-inv-sold-out={item.soldOut}
									>
										<div class="mobile-left">
											{#if isMerchTableInventory(item)}
												{#if (item.productCode ?? '').trim()}
													<div class="mobile-product-code merch-table-code">{item.productCode}</div>
												{/if}
												<em class="unique-item-note">*Merch Table</em>
											{:else}
												<div class="mobile-product-code">{item.productCode || '—'}</div>
											{/if}
										</div>
										<div class="mobile-center">
											<div class="mobile-item-name">{item.title}</div>
											{#if (item.artistName ?? '').trim()}
												<div class="mobile-artist">By {item.artistName}</div>
											{/if}
											{#if item.itemType}
												<div class="mobile-item-type">{item.itemType}</div>
											{/if}
										</div>
										<div class="mobile-prices">
											<div class="mobile-price-cad">{formatCents(item.priceCents)}</div>
										</div>
										<div class="mobile-action">
											<div
												class="staff-inv-sold-menu-wrap"
												class:open={soldOutMenuItemId === item.id}
											>
												<button
													type="button"
													class="staff-inv-sold-trigger"
													class:open={soldOutMenuItemId === item.id}
													class:is-sold-out={item.soldOut}
													disabled={savingSoldOutId === item.id}
													aria-expanded={soldOutMenuItemId === item.id}
													aria-haspopup="menu"
													on:click={() => toggleSoldOutMenu(item.id)}
												>
													<span class="staff-inv-sold-label">
														{#if savingSoldOutId === item.id}
															Saving…
														{:else if item.soldOut}
															Sold Out
														{:else}
															<span class="staff-inv-avail-full">Available</span>
															<span class="staff-inv-avail-short">Avail.</span>
														{/if}
													</span>
													<span class="staff-paid-caret" aria-hidden="true"></span>
												</button>
												{#if soldOutMenuItemId === item.id}
													<button
														type="button"
														class="staff-paid-menu-backdrop"
														aria-label="Close availability menu"
														on:click={closeSoldOutMenu}
													></button>
													<div class="staff-paid-menu staff-inv-sold-menu" role="menu">
														<button
															type="button"
															role="menuitem"
															disabled={savingSoldOutId === item.id}
															on:click={() => toggleSoldOut(item)}
														>
															{item.soldOut ? 'Mark as Available' : 'Mark as Sold Out'}
														</button>
													</div>
												{/if}
											</div>
										</div>
									</div>
								{/each}
							</section>
						{/each}
					</div>
				</div>
			{/if}
		</section>
	{/if}

	{#if prepareConfirmOpen}
		<div class="staff-confirm-overlay" role="presentation">
			<button
				type="button"
				class="staff-confirm-backdrop"
				aria-label="Cancel prepare confirmation"
				on:click={closePrepareConfirm}
			></button>
			<div
				class="staff-confirm-dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="staff-prepare-confirm-title"
			>
				<button
					type="button"
					class="staff-confirm-close"
					aria-label="Cancel"
					on:click={closePrepareConfirm}
				>
					×
				</button>
				<p id="staff-prepare-confirm-title" class="staff-confirm-message">
					Did you write the order number on the shopping bag?
				</p>
				{#if selectedOrder}
					<p class="staff-confirm-order-id" aria-hidden="true">
						{orderIdBase(selectedOrder.orderId, selectedOrder.merchTableOrder)}{#if orderIdHasMerchSuffix(selectedOrder.orderId, selectedOrder.merchTableOrder)}<span
								class="staff-merch-m">M</span
							>{/if}
					</p>
				{/if}
				<button
					type="button"
					class="staff-confirm-btn"
					data-status="prepared"
					disabled={actionsLocked}
					on:click={confirmMarkPrepared}
				>
					Yes! The Order is Prepared
				</button>
			</div>
		</div>
	{/if}

	{#if invoicedConfirmOpen}
		<div class="staff-confirm-overlay" role="presentation">
			<button
				type="button"
				class="staff-confirm-backdrop"
				aria-label="Cancel set as invoiced"
				on:click={closeInvoicedConfirm}
			></button>
			<div
				class="staff-confirm-dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="staff-invoiced-confirm-title"
			>
				<p id="staff-invoiced-confirm-title" class="staff-confirm-message">
					Setting as Invoiced does not undo any notifications or emails already sent out.
				</p>
				<div class="staff-confirm-actions">
					<button
						type="button"
						class="staff-confirm-btn staff-confirm-btn-secondary"
						disabled={actionsLocked}
						on:click={closeInvoicedConfirm}
					>
						Cancel
					</button>
					<button
						type="button"
						class="staff-confirm-btn"
						data-status="invoiced"
						disabled={actionsLocked}
						on:click={confirmSetAsInvoiced}
					>
						Set As Invoiced
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if discordNotifyOpen && selectedOrder?.discordHandle.trim()}
		<div class="staff-confirm-overlay" role="presentation">
			<button
				type="button"
				class="staff-confirm-backdrop"
				aria-label="Close Discord notify"
				on:click={closeDiscordNotify}
			></button>
			<div
				class="staff-confirm-dialog staff-discord-notify-dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="staff-discord-notify-title"
			>
				<button
					type="button"
					class="staff-confirm-close"
					aria-label="Close"
					on:click={closeDiscordNotify}
				>
					×
				</button>
				<p id="staff-discord-notify-title" class="staff-confirm-message staff-confirm-title">
					Notify on Discord
				</p>
				<p class="staff-discord-notify-hint">
					Copy this message and notify the user in discord.
				</p>
				<div class="staff-notify-sample">
					<pre class="staff-notify-message">{discordNotifyMessage(selectedOrder)}</pre>
					<button
						type="button"
						class="staff-copy-icon-btn"
						class:copied={copyFlash === 'Copied'}
						disabled={actionsLocked}
						aria-label={copyFlash === 'Copied' ? 'Copied' : 'Copy message'}
						title={copyFlash === 'Copied' ? 'Copied' : 'Copy'}
						on:click={copyDiscordNotifyMessage}
					>
						{#if copyFlash === 'Copied'}
							<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none">
								<path
									d="M5 13l4 4L19 7"
									stroke="currentColor"
									stroke-width="2.25"
									stroke-linecap="round"
									stroke-linejoin="round"
								/>
							</svg>
						{:else}
							<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none">
								<rect
									x="8"
									y="8"
									width="12"
									height="12"
									rx="2"
									stroke="currentColor"
									stroke-width="2"
								/>
								<path
									d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"
									stroke="currentColor"
									stroke-width="2"
									stroke-linecap="round"
									stroke-linejoin="round"
								/>
							</svg>
						{/if}
					</button>
				</div>
				<div class="staff-confirm-actions staff-discord-notify-actions">
					<button
						type="button"
						class="staff-confirm-btn staff-confirm-btn-secondary staff-discord-notify-cancel"
						disabled={actionsLocked}
						on:click={closeDiscordNotify}
					>
						Cancel
					</button>
					<button
						type="button"
						class="staff-confirm-btn staff-discord-notify-confirm"
						data-status="notified"
						disabled={actionsLocked}
						on:click={() => markNotified('discord')}
					>
						<svg viewBox="0 0 24 24" width="18" height="18" aria-hidden="true" fill="none">
							<path
								d="M5 13l4 4L19 7"
								stroke="currentColor"
								stroke-width="2.5"
								stroke-linecap="round"
								stroke-linejoin="round"
							/>
						</svg>
						Customer Notified!
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if cancelConfirmOpen}
		<div class="staff-confirm-overlay" role="presentation">
			<button
				type="button"
				class="staff-confirm-backdrop"
				aria-label="Cancel close order confirmation"
				on:click={closeCancelConfirm}
			></button>
			<div
				class="staff-confirm-dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="staff-cancel-confirm-title"
			>
				<p id="staff-cancel-confirm-title" class="staff-confirm-message staff-confirm-title">
					Are you sure?
				</p>
				<p class="staff-confirm-detail">
					Are you sure you want to close this order? This will not undo any payments or
					emails that have already been sent.
				</p>
				<div class="staff-confirm-actions">
					<button
						type="button"
						class="staff-confirm-btn staff-confirm-btn-secondary"
						disabled={actionsLocked}
						on:click={closeCancelConfirm}
					>
						Cancel
					</button>
					<button
						type="button"
						class="staff-confirm-btn"
						data-status="closed"
						disabled={actionsLocked}
						on:click={confirmCancelOrder}
					>
						Close Order
					</button>
				</div>
			</div>
		</div>
	{/if}

	{#if paidAmountConfirmOpen && pendingPaidReason}
		<div class="staff-confirm-overlay" role="presentation">
			<button
				type="button"
				class="staff-confirm-backdrop"
				aria-label="Cancel payment confirmation"
				disabled={actionsLocked}
				on:click={() => {
					if (!actionsLocked) closePaidAmountConfirm();
				}}
			></button>
			<div
				class="staff-confirm-dialog"
				role="dialog"
				aria-modal="true"
				aria-labelledby="staff-paid-amount-confirm-title"
				aria-busy={actionsLocked}
			>
				<p
					id="staff-paid-amount-confirm-title"
					class="staff-confirm-message staff-confirm-title"
				>
					{paidConfirmTitle(pendingPaidReason)}
				</p>
				{#if actionsLocked}
					<p class="staff-confirm-detail staff-paid-syncing" role="status">
						Marking paid and syncing with Shopify… This can take a few seconds.
					</p>
					<div class="staff-paid-sync-spinner" aria-hidden="true"></div>
				{:else}
					<p class="staff-confirm-detail">
						{paidConfirmDetail(pendingPaidReason)}
						{#if selectedOrder}
							Order total is {formatCents(collectedTotalCents(selectedOrder))}.
						{/if}
					</p>
					<label class="staff-cash-amount-label">
						Amount (CAD)
						<input
							bind:this={paidAmountInputEl}
							type="text"
							inputmode="decimal"
							autocomplete="off"
							class="staff-cash-amount-input"
							bind:value={paidAmountDraft}
							disabled={actionsLocked}
							on:keydown={(event) => {
								if (event.key === 'Enter') {
									event.preventDefault();
									void confirmManualPaid();
								}
							}}
						/>
					</label>
				{/if}
				<div class="staff-confirm-actions staff-paid-amount-actions">
					<button
						type="button"
						class="staff-confirm-btn staff-confirm-btn-secondary staff-paid-amount-cancel"
						disabled={actionsLocked}
						on:click={closePaidAmountConfirm}
					>
						Cancel
					</button>
					<button
						type="button"
						class="staff-confirm-btn staff-paid-amount-confirm"
						data-status="paid"
						disabled={actionsLocked || parsePaidAmountToCents(paidAmountDraft) == null}
						on:click={confirmManualPaid}
					>
						{actionsLocked ? 'Marking paid…' : paidConfirmActionLabel(pendingPaidReason)}
					</button>
				</div>
			</div>
		</div>
	{/if}
	{/if}
</div>

