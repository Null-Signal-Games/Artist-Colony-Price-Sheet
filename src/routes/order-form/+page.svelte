<script lang="ts">
	import { cart, cartCount, formatMoney, parseMoney, type CartItem } from '$lib/cart';

	let name = '';
	let email = '';
	let phone = '';
	let notes = '';
	let submitted = false;
	let orderId = '';
	let submittedItems: CartItem[] = [];

	$: items = submitted ? submittedItems : $cart;
	$: gbpTotal = items.reduce((total, item) => total + parseMoney(item.gbp) * item.quantity, 0);
	$: usdTotal = items.reduce((total, item) => total + parseMoney(item.usd) * item.quantity, 0);
	$: euroTotal = items.reduce((total, item) => total + parseMoney(item.euro) * item.quantity, 0);

	function submitOrder() {
		submittedItems = $cart.map((item) => ({ ...item }));
		orderId = `AC-${Date.now().toString().slice(-6)}`;
		submitted = true;
		cart.clear();
	}

	function startNewOrder() {
		name = '';
		email = '';
		phone = '';
		notes = '';
		submitted = false;
		orderId = '';
		submittedItems = [];
	}
</script>

<div class="order-form-page">
	<header class="order-form-header">
		<a class="back-link" href="/">← Price Sheet</a>
		<h1>Order Form</h1>
	</header>

	<main class="order-form-main">
		{#if submitted}
			<section class="order-confirmation">
				<h2>Order received</h2>
				<p class="order-id">Order {orderId}</p>
				<p>
					This is not a payment. Please show this confirmation to the cashier when you collect your
					items.
				</p>
				<p><strong>{name}</strong></p>
				<p>{email}{phone ? ` · ${phone}` : ''}</p>
				<ul class="order-items">
					{#each items as item}
						<li>
							<span>{item.quantity}× {item.productCode || item.title}</span>
							<span>{formatMoney(parseMoney(item.gbp) * item.quantity, '£')}</span>
						</li>
					{/each}
				</ul>
				<p class="order-total">
					<strong>Total: {formatMoney(gbpTotal, '£')}</strong>
					<span>{formatMoney(usdTotal, '$')} / {formatMoney(euroTotal, '€')}</span>
				</p>
				{#if notes}
					<p class="order-notes">Notes: {notes}</p>
				{/if}
				<button type="button" class="submit-order-button" on:click={startNewOrder}>
					Start a new order
				</button>
			</section>
		{:else if $cartCount === 0}
			<section class="empty-cart">
				<p>Your cart is empty.</p>
				<a class="submit-order-button" href="/">Browse the price sheet</a>
			</section>
		{:else}
			<section class="order-items-section">
				<h2>Your items</h2>
				<ul class="order-items editable">
					{#each items as item}
						<li>
							<div class="item-details">
								<div class="item-title">{item.title}</div>
								<div class="item-meta">
									{item.productCode}{item.productCode && item.artist ? ' · ' : ''}{item.artist}
								</div>
								<div class="item-price">{item.gbp}</div>
							</div>
							<div class="item-actions">
								<div class="qty-controls">
									<button
										type="button"
										aria-label="Decrease quantity"
										on:click={() => cart.setQuantity(item.id, item.quantity - 1)}
									>
										−
									</button>
									<span>{item.quantity}</span>
									<button
										type="button"
										aria-label="Increase quantity"
										on:click={() => cart.setQuantity(item.id, item.quantity + 1)}
									>
										+
									</button>
								</div>
								<button type="button" class="remove-item" on:click={() => cart.remove(item.id)}>
									Remove
								</button>
							</div>
						</li>
					{/each}
				</ul>
				<div class="order-total">
					<strong>Total: {formatMoney(gbpTotal, '£')}</strong>
					<span>{formatMoney(usdTotal, '$')} / {formatMoney(euroTotal, '€')}</span>
				</div>
			</section>

			<form class="order-details-form" on:submit|preventDefault={submitOrder}>
				<h2>Your details</h2>
				<label>
					Name
					<input type="text" bind:value={name} required autocomplete="name" />
				</label>
				<label>
					Email
					<input type="email" bind:value={email} required autocomplete="email" />
				</label>
				<label>
					Phone <span class="optional">(optional)</span>
					<input type="tel" bind:value={phone} autocomplete="tel" />
				</label>
				<label>
					Notes <span class="optional">(optional)</span>
					<textarea bind:value={notes} rows="3" placeholder="Pickup notes, sizes, or special requests"></textarea>
				</label>
				<p class="payment-note">No payment is taken here. Submit this form, then check out with the cashier.</p>
				<button type="submit" class="submit-order-button">Submit order</button>
			</form>
		{/if}
	</main>
</div>
