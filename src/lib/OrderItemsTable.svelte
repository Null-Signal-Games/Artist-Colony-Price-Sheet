<script lang="ts">
	import { cart, formatMoney, parseMoney, type CartItem } from '$lib/cart';

	export let items: CartItem[];
	export let editable = false;

	function currencySymbol(value: string) {
		const match = value.trim().match(/^[^\d.-]+/);
		return match ? match[0] : '$';
	}

	function lineSubtotal(item: CartItem) {
		return formatMoney(parseMoney(item.gbp) * item.quantity, currencySymbol(item.gbp));
	}
</script>

<div class="order-items-table">
	<div class="order-items-header">
		<div class="col-item">Item</div>
		<div class="col-price">Price</div>
		<div class="col-qty">Quantity</div>
		<div class="col-subtotal">Subtotal</div>
	</div>
	<ul class="order-items">
		{#each items as item}
			<li>
				<div class="item-title">{item.title}</div>
				<div class="item-price">{item.gbp}</div>
				{#if editable}
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
				{:else}
					<div class="item-qty">{item.quantity}</div>
				{/if}
				<div class="item-subtotal">{lineSubtotal(item)}</div>
				<div class="item-meta">
					{item.productCode}{item.productCode && item.artist ? ' · ' : ''}{item.artist}
				</div>
				{#if editable}
					<button type="button" class="remove-item" on:click={() => cart.remove(item.id)}>
						Remove
					</button>
				{/if}
			</li>
		{/each}
	</ul>
</div>
