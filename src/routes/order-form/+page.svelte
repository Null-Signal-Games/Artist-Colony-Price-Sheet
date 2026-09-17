<script lang="ts">
	import OrderItemsTable from '$lib/OrderItemsTable.svelte';
	import { cart, cartCount, formatMoney, parseMoney, type CartItem } from '$lib/cart';

	let name = '';
	let discordHandle = '';
	let email = '';
	let submitted = false;
	let timestamp = '';
	let submittedItems: CartItem[] = [];

	$: items = submitted ? submittedItems : $cart;
	$: gbpTotal = items.reduce((total, item) => total + parseMoney(item.gbp) * item.quantity, 0);
	$: currencySymbol = items[0]?.gbp.trim().match(/^[^\d.-]+/)?.[0] ?? '$';

	function formatTimestamp(date: Date) {
		return date.toLocaleString('en-GB', {
			timeZone: 'Europe/London',
			dateStyle: 'full',
			timeStyle: 'short'
		});
	}

	function submitOrder(event: Event) {
		const form = event.currentTarget as HTMLFormElement;
		const nameInput = form.querySelector<HTMLInputElement>('input[autocomplete="name"]');
		const emailInput = form.querySelector<HTMLInputElement>('input[type="email"]');

		nameInput?.setCustomValidity(name.trim() ? '' : 'Please enter your name');
		emailInput?.setCustomValidity(email.trim() ? '' : 'Please enter your email');

		if (!form.checkValidity()) {
			form.reportValidity();
			return;
		}

		name = name.trim();
		email = email.trim();
		discordHandle = discordHandle.trim();
		submittedItems = $cart.map((item) => ({ ...item }));
		timestamp = formatTimestamp(new Date());
		submitted = true;
		cart.clear();
	}
</script>

<div class="order-form-page">
	<header class="order-form-header">
		<a class="back-link" href="/">← View Items</a>
		<h1>Artist Colony</h1>
	</header>

	<main class="order-form-main">
		{#if submitted}
			<div class="order-confirmation-wrap">
				<section class="order-confirmation">
					<h2 class="order-page-title">Order Submitted!</h2>
					<p class="order-timestamp">{timestamp}</p>
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

					<OrderItemsTable items={items} />
					<div class="order-total">
						<strong>*Subtotal: {formatMoney(gbpTotal, currencySymbol)}</strong>
					</div>

					<p class="tax-note">
						*Subtotal: Tax will added to your total in the final invoice that will be sent to you.
					</p>
					<p class="payment-note">
						Your order will be processed in the order it is received. You will be notified via Discord
						&amp; Email when an invoice has been created for you. Pay ASAP. Unpaid invoices expire
						after 2 hours and items will be returned to inventory. Orders must be paid in CAD.
					</p>
				</section>
				<a class="submit-order-button" href="/">View Price Sheet</a>
			</div>
		{:else if $cartCount === 0}
			<section class="empty-cart">
				<h2 class="order-page-title">Order Form</h2>
				<p>Your cart is empty.</p>
				<a class="submit-order-button" href="/">View items</a>
			</section>
		{:else}
			<form class="order-details-form" on:submit|preventDefault={submitOrder}>
				<h2 class="order-page-title">Order Form</h2>
				<label>
					Name*
					<input
						type="text"
						bind:value={name}
						required
						autocomplete="name"
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
						/>
					</label>
					<label>
						Email*
						<input
							type="email"
							bind:value={email}
							required
							autocomplete="email"
							on:input={(event) => event.currentTarget.setCustomValidity('')}
						/>
					</label>
				</div>

				<section class="order-items-section">
					<OrderItemsTable items={items} editable />
					<div class="order-total">
						<strong>*Subtotal: {formatMoney(gbpTotal, currencySymbol)}</strong>
					</div>
				</section>

				<p class="tax-note">
					*Subtotal: Tax will added to your total in the final invoice that will be sent to you.
				</p>
				<p class="payment-note">
					Once you submit your order, it will be processed in the order it is received. You will be
					notified via Discord &amp; Email when an invoice has been created for you. Pay ASAP.
					Unpaid invoices expire after 2 hours and items will be returned to inventory. Orders must
					be paid in CAD.
				</p>
				<button type="submit" class="submit-order-button">Submit order</button>
			</form>
		{/if}
	</main>
</div>
