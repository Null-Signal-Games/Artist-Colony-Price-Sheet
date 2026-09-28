<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { base } from '$app/paths';
	import OrderItemsTable from '$lib/OrderItemsTable.svelte';
	import { cart, cartCount, formatMoney, parseMoney, type CartItem } from '$lib/cart';
	import { consumeCheckoutDraft } from '$lib/checkoutDraft';
	import {
		buildOrderId,
		buildSubtotal,
		submitOrder as submitOrderToServer
	} from '$lib/submitOrder';
	import { getCurrentStaffName, isStaffSession, staffApi } from '$lib/staff/api';

  // default so that we don't need to collect emails at the merch table
	const MERCH_STAFF_EMAIL = 'marketing@nullsignal.games';

	let name = '';
	let discordHandle = '';
	let email = '';
	let submitted = false;
	let submitting = false;
	let submitError = '';
	let timestamp = '';
	let orderId = '';
	let submittedItems: CartItem[] = [];
	let staffSession = false;
	let merchTableOrder = false;

	$: items = submitted ? submittedItems : $cart;
	$: cadTotal = items.reduce((total, item) => total + parseMoney(item.cad) * item.quantity, 0);
	$: customerFieldsRequired = !merchTableOrder;

	onMount(() => {
		staffSession = isStaffSession();
		const draft = consumeCheckoutDraft();
		if (draft) {
			name = draft.name;
			discordHandle = draft.discordHandle;
			email = draft.email;
			merchTableOrder = draft.merchTableOrder;
		}
	});

	function staffNameDefault() {
		return `(${getCurrentStaffName()})`;
	}

	function onMerchTableChange() {
		const staffDefault = staffNameDefault();
		if (merchTableOrder) {
			if (!name.trim()) name = staffDefault;
			if (!email.trim()) email = MERCH_STAFF_EMAIL;
		} else {
			if (name.trim() === staffDefault) name = '';
			if (email.trim() === MERCH_STAFF_EMAIL) email = '';
		}
	}

	function formatTimestamp(date: Date) {
		const parts = new Intl.DateTimeFormat('en-CA', {
			timeZone: 'America/New_York',
			weekday: 'long',
			year: 'numeric',
			month: '2-digit',
			day: '2-digit',
			hour: 'numeric',
			minute: '2-digit',
			hour12: true
		});
		const segments: Record<string, string> = {};
		for (const part of parts.formatToParts(date)) {
			segments[part.type] = part.value;
		}
		return `${segments.weekday} ${segments.year}-${segments.month}-${segments.day} at ${segments.hour}:${segments.minute}${segments.dayPeriod?.replace(/\./g, '').toLowerCase() ?? ''}`;
	}

	async function submitOrder(event: Event) {
		const form = event.currentTarget as HTMLFormElement;
		const nameInput = form.querySelector<HTMLInputElement>('input[autocomplete="name"]');
		const emailInput = form.querySelector<HTMLInputElement>('input[type="email"]');

		if (merchTableOrder) {
			nameInput?.setCustomValidity('');
			emailInput?.setCustomValidity('');
		} else {
			nameInput?.setCustomValidity(name.trim() ? '' : 'Please enter your name');
			emailInput?.setCustomValidity(email.trim() ? '' : 'Please enter your email');
		}

		if (!form.checkValidity()) {
			form.reportValidity();
			return;
		}

		name = name.trim() || (merchTableOrder ? staffNameDefault() : '');
		email = email.trim() || (merchTableOrder ? MERCH_STAFF_EMAIL : '');
		discordHandle = discordHandle.trim();
		submitError = '';
		submitting = true;

		const nextOrderId = buildOrderId(new Date(), { merchTableOrder });
		const snapshot = $cart.map((item) => ({ ...item }));
		const staffName = staffSession || isStaffSession() ? getCurrentStaffName() : '';

		try {
			if (staffSession && staffName) {
				const order = await staffApi.createStaffOrder(
					{
						orderId: nextOrderId,
						name,
						discordHandle,
						email,
						items: snapshot.map((item) => ({
							productCode: item.productCode,
							title: item.title,
							artist: item.artist,
							unitPriceCents: Math.round(parseMoney(item.cad) * 100),
							quantity: item.quantity
						})),
						submittedByStaffName: staffName,
						merchTableOrder
					},
					{ staffName }
				);
				/* void submitOrderToServer({
					orderId: order.orderId,
					name,
					discordHandle,
					email,
					items: snapshot,
					subtotal: buildSubtotal(snapshot),
					submittedAt: new Date().toISOString(),
					submittedByStaffName: staffName,
					...(merchTableOrder ? { merchTableOrder: true } : {})
				}); */
				cart.clear();
				await goto(`${base}/staff?order=${encodeURIComponent(order.orderId)}`);
				return;
			}

			const result = await submitOrderToServer({
				orderId: nextOrderId,
				name,
				discordHandle,
				email,
				items: snapshot,
				subtotal: buildSubtotal(snapshot),
				submittedAt: new Date().toISOString()
			});

			if (!result.ok) {
				submitError = result.error;
				return;
			}

			orderId = result.orderId;
			submittedItems = snapshot;
			timestamp = formatTimestamp(new Date());
			submitted = true;
			cart.clear();
		} catch (err) {
			submitError = err instanceof Error ? err.message : 'Could not submit order.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="order-form-page">
	<header class="order-form-header">
		<a class="back-link" href="{base}/">← View Items</a>
		{#if staffSession}
			<a class="title-staff-link" href="{base}/staff">STAFF</a>
		{:else}
			<h1>Artist Colony</h1>
		{/if}
	</header>

	<main class="order-form-main">
		{#if submitted}
			<div class="order-confirmation-wrap">
				<section class="order-confirmation">
					<h2 class="order-page-title">Order Submitted!</h2>
					<p class="order-timestamp">{timestamp}</p>
					{#if orderId}
						<p class="order-id">Order {orderId}</p>
					{/if}
					{#if merchTableOrder}
						<p class="order-merch-flag">Merch Table Order</p>
					{/if}
					<dl class="order-fields">
						<div>
							<dt>Name</dt>
							<dd>{name}</dd>
						</div>
						<div class="form-row">
							<div>
								<dt>Discord Handle</dt>
								<dd>{discordHandle || 'N/A'}</dd>
							</div>
							<div>
								<dt>Email</dt>
								<dd>{email}</dd>
							</div>
						</div>
					</dl>

					<OrderItemsTable {items} />
					<div class="order-total">
						<strong>Total: {formatMoney(cadTotal, '$')} CAD</strong>
					</div>

					<p class="payment-note">
						Your order will be processed in the order it is received. You will be notified via
						Discord &amp; Email when an invoice has been created for you. Pay ASAP. Unpaid invoices
						expire after 2 hours and items will be returned to inventory. Orders must be paid in
						CAD.
					</p>
				</section>
				<a class="submit-order-button" href="{base}/">View Price Sheet</a>
			</div>
		{:else if $cartCount === 0}
			<section class="empty-cart">
				<h2 class="order-page-title">Order Form</h2>
				<p>Your cart is empty.</p>
				<a class="submit-order-button" href="{base}/">View items</a>
			</section>
		{:else}
			<form class="order-details-form" on:submit|preventDefault={submitOrder}>
				<h2 class="order-page-title">Order Form</h2>
				{#if staffSession}
					<label class="merch-table-order-check">
						<input
							type="checkbox"
							bind:checked={merchTableOrder}
							disabled={submitting}
							on:change={onMerchTableChange}
						/>
						<span>Merch Table Order</span>
					</label>
				{/if}
				<label>
					{customerFieldsRequired ? 'Name*' : 'Name'}
					<input
						type="text"
						bind:value={name}
						required={customerFieldsRequired}
						autocomplete="name"
						disabled={submitting}
						on:input={(event) => event.currentTarget.setCustomValidity('')}
					/>
				</label>
				<div class="form-row">
					<label>
						Discord Handle
						<input
							type="text"
							bind:value={discordHandle}
							autocomplete="username"
							placeholder="@username"
							disabled={submitting}
						/>
					</label>
					<label>
						{customerFieldsRequired ? 'Email*' : 'Email'}
						<input
							type="email"
							bind:value={email}
							required={customerFieldsRequired}
							autocomplete="email"
							disabled={submitting}
							on:input={(event) => event.currentTarget.setCustomValidity('')}
						/>
					</label>
				</div>

				<section class="order-items-section">
					<OrderItemsTable {items} editable={!submitting} />
					<div class="order-total">
						<strong>Total: {formatMoney(cadTotal, '$')} CAD</strong>
					</div>
				</section>

				<p class="payment-note">
					Once you submit your order, it will be processed in the order it is received. You will be
					notified via Discord &amp; Email when an invoice has been created for you. Pay ASAP.
					Unpaid invoices expire after 2 hours and items will be returned to inventory. Orders must
					be paid in CAD.
				</p>
				{#if submitError}
					<p class="submit-error" role="alert">{submitError}</p>
				{/if}
				<button type="submit" class="submit-order-button" disabled={submitting}>
					{submitting ? 'Submitting…' : 'Submit order'}
				</button>
			</form>
		{/if}
	</main>
</div>
