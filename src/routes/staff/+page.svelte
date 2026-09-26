<script lang="ts">
	import { goto, replaceState } from '$app/navigation';
	import { page } from '$app/stores';
	import { onMount, tick } from 'svelte';
	import { get } from 'svelte/store';

	import {
		CLOSED_REASON_LABELS,
		CLOSED_REASONS,
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
		type PaidReason
	} from '$lib/staff/api';
	import {
		defaultStatusChecks,
		readStoredFilterState,
		writeStoredFilterState
	} from '$lib/staff/filterState';

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

	let statusChecks = defaultStatusChecks(true);
	let orderSort: OrderSort = 'oldest';
	let orderQuery = '';
	let orders: OrderSummary[] = [];
	let selectedOrderId = '';
	let selectedOrder: Order | null = null;
	let notesDraft = '';
	let closedReasonOtherDraft = '';
	let paidMenuOpen = false;
	let statusMenuOpen = false;
	let prepareConfirmOpen = false;
	let invoicedConfirmOpen = false;

	let inventoryQuery = '';
	let inventory: InventoryItem[] = [];

	let menuOpen = false;
	let statusFilterOpen = false;
	let searchOpen = false;
	let searchInputEl: HTMLInputElement | null = null;
	let openLineMenuIndex: number | null = null;
	let sendingInvoice = false;
	let invoiceSuccess = '';
	let copyFlash = '';

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

	async function loadOrders() {
		loading = true;
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
				closedReasonOtherDraft = selectedOrder?.closedReasonOther ?? '';
				if (!selectedOrder) {
					await clearSelectedOrder({ skipFlush: true });
				}
			}
		} catch (err) {
			if (err instanceof StaffAuthError) {
				authed = false;
				loginError = err.message;
			} else {
				error = err instanceof Error ? err.message : 'Failed to load orders.';
			}
		} finally {
			loading = false;
		}
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
		try {
			selectedOrder = await staffApi.getOrder(orderId);
			if (!selectedOrder) {
				await clearSelectedOrder({ skipFlush: true });
				error = 'Order not found.';
				return;
			}
			notesDraft = selectedOrder.staffNotes ?? '';
			closedReasonOtherDraft = selectedOrder.closedReasonOther ?? '';
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
		closedReasonOtherDraft = '';
		paidMenuOpen = false;
		statusMenuOpen = false;
		prepareConfirmOpen = false;
		invoicedConfirmOpen = false;
		openLineMenuIndex = null;
		invoiceSuccess = '';
		statusFilterOpen = false;
		searchOpen = false;
		syncSelectedOrderUrl('');
	}

	async function setStatus(
		status: OrderStatus,
		options: {
			closedReason?: ClosedReason;
			paidReason?: PaidReason;
		} = {}
	) {
		if (!selectedOrder || actionsLocked) return;
		if (!canSelectStatus(status) && status !== selectedOrder.status) return;
		if (!(await flushNotesIfNeeded({ quiet: true }))) return;
		saving = true;
		error = '';
		invoiceSuccess = '';
		try {
			const closedReason = options.closedReason ?? 'canceled';
			const paidReason = options.paidReason ?? 'cash';
			const meta = staffMeta();
			selectedOrder = await staffApi.updateOrderStatus(
				selectedOrder.orderId,
				status,
				status === 'closed'
					? {
							...meta,
							closedReason,
							closedReasonOther:
								closedReason === 'other' ? closedReasonOtherDraft.trim() : undefined
						}
					: status === 'paid'
						? {
								...meta,
								paidReason
							}
						: status === 'notified'
							? {
									...meta,
									notificationChannel:
										selectedOrder.notificationChannel ?? undefined
								}
							: meta
			);
			closedReasonOtherDraft = selectedOrder.closedReasonOther ?? '';
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update status.';
		} finally {
			saving = false;
		}
	}

	async function setClosedReason(reason: ClosedReason) {
		if (!selectedOrder || selectedOrder.status !== 'closed' || actionsLocked) return;
		if (
			selectedOrder.closedReason === 'canceled' ||
			selectedOrder.closedReason === 'refunded'
		) {
			return;
		}
		saving = true;
		error = '';
		try {
			selectedOrder = await staffApi.updateOrderStatus(selectedOrder.orderId, 'closed', {
				...staffMeta(),
				closedReason: reason,
				closedReasonOther: reason === 'other' ? closedReasonOtherDraft.trim() : undefined
			});
			closedReasonOtherDraft = selectedOrder.closedReasonOther ?? '';
			await refreshOrdersList();
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update closed reason.';
		} finally {
			saving = false;
		}
	}

	/* async function saveClosedReasonOther() {
		if (!selectedOrder || selectedOrder.status !== 'closed' || actionsLocked) return;
		if (selectedOrder.closedReason !== 'other') return;
		await setClosedReason('other');
	} */

	async function markPaid(reason: Exclude<PaidReason, 'shopify'>) {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status !== 'notified') return;
		error = '';
		await setStatus('paid', {
			paidReason: reason
		});
		closePaidMenu();
		closeStatusMenu();
	}

	async function choosePaidReason(reason: PaidReason) {
		if (reason === 'shopify' || actionsLocked) return;
		closeStatusMenu();
		if (selectedOrder?.status === 'notified') {
			await markPaid(reason);
		}
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
				selectedOrder.closedReason === 'refunded' ||
				selectedOrder.closedReason === 'canceled');
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

	async function cancelOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (selectedOrder.status === 'closed') return;
		closeStatusMenu();
		closePaidMenu();
		await setStatus('closed', { closedReason: 'canceled' });
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

	function isLineReadyForPrepare(item: OrderItem) {
		return item.lineAction === 'sold_out' || item.collected;
	}

	/* function orderWasPaid(order: Order) {
		return order.status === 'paid' || Boolean(order.paidReason);
	}
  */

	$: allLinesReadyForPrepare = selectedOrder
		? selectedOrder.items.length > 0 && selectedOrder.items.every(isLineReadyForPrepare)
		: false;

	$: paidViaShopify = !!selectedOrder && selectedOrder.paidReason === 'shopify';

	$: paidMethodLabel = selectedOrder?.paidReason
		? PAID_REASON_LABELS[selectedOrder.paidReason]
		: 'Paid';

	$: canReopenClosed =
		!!selectedOrder && selectedOrder.status === 'closed';

	$: isPickedUp =
		!!selectedOrder &&
		selectedOrder.status === 'closed' &&
		selectedOrder.closedReason === 'picked_up';

	$: isRefunded =
		!!selectedOrder &&
		selectedOrder.status === 'closed' &&
		selectedOrder.closedReason === 'refunded';

	$: isCanceled =
		!!selectedOrder &&
		selectedOrder.status === 'closed' &&
		selectedOrder.closedReason === 'canceled';

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
			return next === 'paid';
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

	async function unsetPaymentType() {
		if (!selectedOrder || selectedOrder.status !== 'paid' || actionsLocked) return;
		if (selectedOrder.paidReason === 'shopify') return;
		closePaidMenu();
		await setStatus('notified');
	}

	async function reopenClosedOrder() {
		if (!selectedOrder || actionsLocked) return;
		if (!canReopenClosed) return;
		closeStatusMenu();
		closePaidMenu();
		await setStatus('paid', {
			paidReason: selectedOrder.paidReason ?? 'cash'
		});
	}

	function discordNotifyMessage(order: Order) {
		const handle = order.discordHandle.trim().replace(/^@/, '');
		return `@${handle} Your Artist Colony Invoice has been emailed to you. Please pay ASAP.`;
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
		} catch (err) {
			error = err instanceof Error ? err.message : 'Failed to update collected quantity.';
		} finally {
			saving = false;
		}
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

<div class="staff-page">
	{#if !authed}
		<section class="staff-panel staff-login-panel">
			<h2 class="staff-login-title">Staff Sign In</h2>
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
					<button type="submit" class="staff-confirm-btn" disabled={loginBusy || !loginUsername.trim() || !loginPassword}>
						{loginBusy ? 'Signing in…' : 'Sign In'}
					</button>
				</form>
			{:else}
				<p class="staff-muted">Loading…</p>
			{/if}
		</section>
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
					class="staff-search-wrap"
					class:open={searchOpen}
					class:has-query={inventoryQuery.trim().length > 0}
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
				<a href="/" on:click={closeMenu}>Price List</a>
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
											>{orderDisplayLabel(order)}</span
										>
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
													on:click={cancelOrder}
												>
													Cancel Order
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
													on:click={cancelOrder}
												>
													Cancel Order
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
													on:click={() =>
																									markNotified(
																										selectedOrder?.discordHandle.trim() ? 'discord' : 'email'
																									)}
												>
													{selectedOrder.discordHandle.trim()
														? 'Notified On Discord'
														: 'Notified Via Email Only'}
												</button>
											{/if}
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked || sendingInvoice}
												on:click={cancelOrder}
											>
												Cancel Order
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
										disabled={actionsLocked}
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
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked}
												on:click={cancelOrder}
											>
												Cancel Order
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
										<span>{paidMethodLabel}</span>
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
											{#if !paidViaShopify}
												<button
													type="button"
													role="menuitem"
													class="staff-status-menu-back"
													disabled={actionsLocked}
													on:click={unsetPaymentType}
												>
													Undo Payment
												</button>
											{/if}
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel staff-status-menu-divider"
												disabled={actionsLocked}
												on:click={refundOrder}
											>
												Mark As Refunded
											</button>
											<button
												type="button"
												role="menuitem"
												class="staff-status-menu-cancel"
												disabled={actionsLocked}
												on:click={cancelOrder}
											>
												Cancel Order
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
												class="staff-status-menu-back"
												disabled={actionsLocked}
												on:click={reopenClosedOrder}
											>
												Re-Open Order
											</button>
										</div>
									{/if}
								</div>
							{:else if isRefunded}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="refunded"
										class:open={statusMenuOpen}
										disabled={actionsLocked}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>Refunded</span>
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
												class="staff-status-menu-back"
												disabled={actionsLocked}
												on:click={reopenClosedOrder}
											>
												Re-Open Order
											</button>
										</div>
									{/if}
								</div>
							{:else if isCanceled}
								<div class="staff-status-menu-wrap">
									<button
										type="button"
										class="staff-status-trigger"
										data-status="canceled"
										class:open={statusMenuOpen}
										disabled={actionsLocked}
										aria-expanded={statusMenuOpen}
										aria-haspopup="menu"
										on:click={toggleStatusMenu}
									>
										<span>Canceled</span>
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
												class="staff-status-menu-back"
												disabled={actionsLocked}
												on:click={reopenClosedOrder}
											>
												Re-Open Order
											</button>
										</div>
									{/if}
								</div>
							{:else if selectedOrder.status === 'closed'}
								<span
									class="staff-closed-badge"
									data-status={orderDisplayStatusKey(selectedOrder)}
								>
									{orderDisplayLabel(selectedOrder)}
								</span>
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

					{#if canReopenClosed && selectedOrder.closedReason === 'other'}
						<div class="staff-closed-block" role="group" aria-label="Re-open closed order">
							<p class="staff-prepare-hint">Re-open sets the order back to Paid.</p>
							<button
								type="button"
								class="staff-prepare-btn"
								disabled={actionsLocked}
								on:click={reopenClosedOrder}
							>
								Re-open
							</button>
						</div>
					{/if}

					{#if selectedOrder.status === 'invoiced' && !selectedOrder.merchTableOrder}
						<div class="staff-notify-block">
							{#if invoiceSuccess}
								<p class="staff-invoice-success" role="status">{invoiceSuccess}</p>
							{/if}
							{#if selectedOrder.discordHandle.trim()}
								<div class="staff-notify-notice" role="status">
									<p class="staff-notify-notice-text">
										Copy this message and notify the customer on Discord. Then set the order to
										Notified On Discord.
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
								</div>
							{:else}
								<p class="staff-notify-notice" role="status">
									This customer did not provide a Discord handle. After confirming they received
									the invoice email, use the Invoiced menu to mark Notified Via Email Only.
								</p>
							{/if}
						</div>
					{:else if invoiceSuccess}
						<p class="staff-invoice-success" role="status">{invoiceSuccess}</p>
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
									<li
										class="staff-line"
										class:staff-row-sold-out={item.lineAction === 'sold_out'}
										class:staff-row-partial={item.lineAction === 'partial'}
										role="row"
									>
										{#if !lineItemsHardLocked}
											<label class="staff-line-col-collected">
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

										<div class="staff-line-col-item staff-line-body">
											<div class="staff-strong">{item.title}</div>
											<div class="staff-muted">
												{item.productCode || 'No code'} · {item.artist}
											</div>
											{#if item.lineAction === 'sold_out'}
												<div class="staff-sold-out-label">Sold out</div>
											{:else if item.lineAction === 'partial'}
												<div class="staff-partial-label">
													Partially Collected{#if item.collectedQuantity != null}
														({item.collectedQuantity})
													{/if}
												</div>
											{:else if lineItemsHardLocked && item.collected}
												<div class="staff-partial-label">Collected</div>
											{/if}
										</div>

										<div class="staff-line-col-qty" aria-label={`Quantity ${qty.effective}`}>
											{#if qty.adjusted}
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
																class:active={item.lineAction === 'partial'}
																disabled={actionsLocked}
																on:click={() =>
																	chooseLineAction(lineIndex, 'partial', item.lineAction)}
															>
																Partially Collected
															</button>
															<button
																type="button"
																role="menuitem"
																class:active={item.lineAction === 'sold_out'}
																disabled={actionsLocked}
																on:click={() =>
																	chooseLineAction(lineIndex, 'sold_out', item.lineAction)}
															>
																Mark As Sold Out
															</button>
														</div>
													{/if}
												</div>
												{#if item.lineAction === 'partial'}
													<label class="staff-partial-qty">
														<span class="staff-line-mobile-label">Collected qty</span>
														<input
															type="number"
															min="0"
															max={item.quantity}
															inputmode="numeric"
															disabled={actionsLocked}
															value={item.collectedQuantity ?? ''}
															aria-label={`Collected quantity for ${item.title}`}
															on:change={(event) =>
																setCollectedQuantity(lineIndex, event.currentTarget.value)}
														/>
													</label>
												{/if}
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
		<section class="staff-panel">
			<p class="staff-muted staff-inventory-hint">
				Server inventory ({inventory.length} products).
			</p>
			{#if loading && !inventory.length}
				<p class="staff-muted">Loading inventory…</p>
			{:else if !inventory.length}
				<p class="staff-muted">No inventory items match.</p>
			{:else}
				<ul class="staff-card-list">
					{#each inventory as item}
						<li class="staff-inv-card" class:staff-row-sold-out={item.soldOut}>
							<div class="staff-card-top">
								<span class="staff-strong">{item.productCode || '—'}</span>
								{#if item.soldOut}
									<span class="staff-sold-out-label">Sold out</span>
								{:else}
									<span class="staff-muted">Available</span>
								{/if}
							</div>
							<div class="staff-strong">{item.title}</div>
							<div class="staff-muted">
								{item.shopName} · {item.itemType} · {formatCents(item.priceCents)}
							</div>
							<div class="staff-inv-meta">
								<span>{item.productDisplay}</span>
								<span>Qty {item.quantity == null ? '—' : item.quantity}</span>
							</div>
						</li>
					{/each}
				</ul>
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
	{/if}
</div>

