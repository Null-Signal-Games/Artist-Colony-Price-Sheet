<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { browser } from '$app/environment';
	import { cart, cartCount, cartItemId, parseMoney } from '$lib/cart';
	import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';
	import { shopPromoDetails } from '$lib/artistDetails';

	export let data: {
		lastUpdated: string;
		csvData: { [key: string]: string }[];
		shopNames?: Set<string>;
		artistNames: Set<string>;
		artistPromos?: Record<string, { label: string; href?: string }[]>;
	};

	let selectedShop = '';
	let searchTerm = '';

	const shopColumnKey = 'Shop Name';
	const artistColumnKey = 'Artist Name';
	const productCodeKey = 'Product Code';
	const productTitleKey = 'Item Name';
	const productDisplayKey = 'Product Display';
	const productTypeKey = 'Item Type';
	const priceKey = 'Price per unit (C$ CAD)';

	const shopOrder = new Map<string, number>();
	const rowOrder = new Map<{ [key: string]: string }, number>();
	data.csvData.forEach((row, index) => {
		rowOrder.set(row, index);
		const shop = row[shopColumnKey] || row[artistColumnKey];
		if (shop && !shopOrder.has(shop)) shopOrder.set(shop, index);
	});

	const shopOptions = Array.from(data.shopNames ?? data.artistNames).sort((a, b) => {
		const orderA = shopOrder.get(a) ?? Number.MAX_SAFE_INTEGER;
		const orderB = shopOrder.get(b) ?? Number.MAX_SAFE_INTEGER;
		if (orderA !== orderB) return orderA - orderB;
		return a.localeCompare(b);
	});

	function isMerchTableItem(row: { [key: string]: string }) {
		return (row[productDisplayKey] ?? '').trim().toLowerCase() === 'merch table';
	}

	function isArtistDisplayItem(row: { [key: string]: string }) {
		return (row[productDisplayKey] ?? '').trim().toLowerCase() === 'artist display';
	}

	function isSoldOutItem(row: { [key: string]: string }) {
		return /sold out/i.test(row[productTitleKey] ?? '');
	}

	function isNotesOnlyItem(row: { [key: string]: string }) {
		const display = (row[productDisplayKey] ?? '').trim().toLowerCase();
		if (display === 'notes') return true;
		return !(row[productCodeKey] ?? '').trim() || !parseMoney(row[priceKey]);
	}

	function canAddToCart(row: { [key: string]: string }) {
		return isArtistDisplayItem(row) && !isNotesOnlyItem(row) && !isSoldOutItem(row);
	}

	function shopName(row: { [key: string]: string }) {
		return (row[shopColumnKey] || row[artistColumnKey] || '').trim();
	}

	$: filteredData = data.csvData
		.filter((row) => {
			const term = searchTerm.trim().toLowerCase();
			const title = (row[productTitleKey] ?? '').toLowerCase();
			const shop = shopName(row).toLowerCase();
			const matchesSearch =
				!term ||
				title.includes(term) ||
				(row[productCodeKey] && row[productCodeKey].toLowerCase().includes(term)) ||
				(row[artistColumnKey] && row[artistColumnKey].toLowerCase().includes(term)) ||
				shop.includes(term);

			return matchesSearch && (!selectedShop || shopName(row) === selectedShop);
		})
		.sort((a, b) => {
			const shopA = shopName(a);
			const shopB = shopName(b);
			const orderA = shopOrder.get(shopA) ?? Number.MAX_SAFE_INTEGER;
			const orderB = shopOrder.get(shopB) ?? Number.MAX_SAFE_INTEGER;
			if (orderA !== orderB) return orderA - orderB;
			return (rowOrder.get(a) ?? 0) - (rowOrder.get(b) ?? 0);
		});

	$: artistGroups = (() => {
		const groups: {
			artist: string;
			details: { label: string; href?: string }[];
			rows: { [key: string]: string }[];
		}[] = [];

		for (const row of filteredData) {
			const shop = shopName(row);
			const last = groups[groups.length - 1];

			if (!last || last.artist !== shop) {
				groups.push({
					artist: shop,
					details: shopPromoDetails(shop, [row], data.artistPromos ?? {}),
					rows: [row]
				});
			} else {
				last.rows.push(row);
				last.details = shopPromoDetails(shop, last.rows, data.artistPromos ?? {});
			}
		}

		return groups;
	})();

	$: if (browser && artistGroups) {
		void tick().then(updateArtistStickyState);
	}

	function updateArtistStickyTop() {
		if (typeof document === 'undefined') return;

		const header = document.querySelector('.fixed-container');
		if (!header) return;

		const navBottom = Math.round(header.getBoundingClientRect().bottom);
		document.documentElement.style.setProperty('--nav-height', `${navBottom}px`);

		let bottom = navBottom;
		const columnHeader = document.querySelector('.fixed-header');

		if (columnHeader && getComputedStyle(columnHeader).display !== 'none') {
			const colRect = columnHeader.getBoundingClientRect();
			bottom = Math.round(colRect.bottom);
			document.documentElement.style.setProperty(
				'--col-header-height',
				`${Math.round(colRect.height)}px`
			);
		}

		document.documentElement.style.setProperty('--artist-sticky-top', `${bottom}px`);
	}

	function updateActiveArtistGroup() {
		if (typeof document === 'undefined') return;

		const stickyTop =
			parseFloat(
				getComputedStyle(document.documentElement).getPropertyValue('--artist-sticky-top')
			) || 0;
		const probeY = stickyTop + 1;
		const groups = [...document.querySelectorAll('table tbody, .mobile-artist-group')].filter(
			(group) => {
				const layout = group.closest('table, .mobile-items');
				return !!layout && getComputedStyle(layout).display !== 'none';
			}
		);

		let active: Element | null = null;
		for (const group of groups) {
			const rect = group.getBoundingClientRect();
			if (rect.top <= probeY && rect.bottom > probeY) {
				active = group;
			}
		}

		document.querySelectorAll('table tbody, .mobile-artist-group').forEach((group) => {
			group.classList.toggle('is-active', group === active);
		});
	}

	function updateArtistStickyState() {
		updateArtistStickyTop();
		updateActiveArtistGroup();
	}

	onMount(() => {
		const header = document.querySelector('.fixed-container');
		const columnHeader = document.querySelector('.fixed-header');
		updateArtistStickyState();

		const observer = new ResizeObserver(updateArtistStickyState);
		if (header) observer.observe(header);
		if (columnHeader) observer.observe(columnHeader);
		window.addEventListener('resize', updateArtistStickyState);
		window.addEventListener('scroll', updateArtistStickyState, { passive: true });

		return () => {
			observer.disconnect();
			window.removeEventListener('resize', updateArtistStickyState);
			window.removeEventListener('scroll', updateArtistStickyState);
		};
	});

	function clearAll() {
		selectedShop = '';
		searchTerm = '';
	}

	function rowId(row: { [key: string]: string }) {
		return cartItemId({ productCode: row[productCodeKey], title: row[productTitleKey] });
	}

	let addedItemIds: Record<string, boolean> = {};
	const addedTimers = new Map<string, ReturnType<typeof setTimeout>>();

	function addToCart(row: { [key: string]: string }) {
		if (!canAddToCart(row)) return;
		const id = rowId(row);
		cart.add({
			id,
			productCode: row[productCodeKey],
			title: row[productTitleKey],
			artist: row[artistColumnKey],
			cad: row[priceKey]
		});

		addedItemIds = { ...addedItemIds, [id]: true };
		const existing = addedTimers.get(id);
		if (existing) clearTimeout(existing);
		addedTimers.set(
			id,
			setTimeout(() => {
				const next = { ...addedItemIds };
				delete next[id];
				addedItemIds = next;
				addedTimers.delete(id);
			}, 1500)
		);
	}
</script>

<div class="fixed-container">
	<div class="title-bar">
		<h1>Artist Colony</h1>
	</div>
	<a class="header-order-link" href="/order-form" aria-label="View order">
		<span class="header-view-order-text">View Order</span>
		<span class="header-cart-icon">
			<svg
				xmlns="http://www.w3.org/2000/svg"
				width="20"
				height="20"
				viewBox="0 0 24 24"
				fill="none"
				stroke="currentColor"
				stroke-width="2"
				stroke-linecap="round"
				stroke-linejoin="round"
				aria-hidden="true"
			>
				<circle cx="9" cy="21" r="1" />
				<circle cx="20" cy="21" r="1" />
				<path d="M1 1h4l2.68 13.39a2 2 0 0 0 2 1.61h9.72a2 2 0 0 0 2-1.61L23 6H6" />
			</svg>
			{#if $cartCount > 0}
				<span class="cart-count">{$cartCount}</span>
			{/if}
		</span>
	</a>
	<div id="filter-fields" class="mb-4 flex items-center">
		<select bind:value={selectedShop} class="border border-gray-300 p-2">
			<option value="">All Shops</option>
			{#each shopOptions as shop}
				<option value={shop}>{shop}</option>
			{/each}
		</select>
		<div class="search-row">
			<input
				type="text"
				placeholder="Search products"
				bind:value={searchTerm}
				class="border border-gray-300 p-2"
			/>
		</div>
		{#if selectedShop || searchTerm}
			<button type="button" on:click={clearAll} class="clear-button" aria-label="Clear filters"
				>&times;</button
			>
		{/if}
		<a class="print-catalogue-link" href="/print">Print</a>
	</div>
</div>
<div class="page-container" class:has-order-cta={$cartCount > 0}>
	<div class="site-footer" class:has-order-cta={$cartCount > 0}>
		{#if $cartCount > 0}
			<a class="view-order-button" href="/order-form">View Order ({$cartCount})</a>
		{/if}
		<div class="footer-notes">
			<p class="merch-note">
				*Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch
				Table.
			</p>
			<p class="last-updated-container">
				Last Updated:
				<span class="last-updated-value font-semibold">{data.lastUpdated}</span>
				EDT (UTC-4)
			</p>
		</div>
	</div>

	{#if filteredData && filteredData.length > 0}
		<!-- Fixed header row -->
		<div class="fixed-header">
			<div class="header-cell artist-col">Artist Name</div>
			<div class="header-cell product-code-col">Product Code</div>
			<div class="header-cell product-col">Product</div>
			<div class="header-cell cad-col">Price (C$)</div>
			<div class="header-cell cart-col"></div>
		</div>

		<!-- Desktop table -->
		<table class="min-w-full">
			<colgroup>
				<col class="col-artist" />
				<col class="col-code" />
				<col class="col-product" />
				<col class="col-cad" />
				<col class="col-cart" />
			</colgroup>
			<thead>
				<tr class="col-sizer">
					<th class="artist-col"></th>
					<th class="product-code-col"></th>
					<th class="product-col"></th>
					<th class="cad-col"></th>
					<th class="cart-col"></th>
				</tr>
			</thead>
			{#each artistGroups as group}
				<tbody>
					<tr class="artist-divider">
						<td colspan="5">
							<ArtistGroupHeading artist={group.artist} details={group.details} />
						</td>
					</tr>
					{#each group.rows as row, i}
						<tr class:stripe-alt={i % 2 === 1} class:notes-only-row={isNotesOnlyItem(row)}>
							<td>
								<span class:small-text={row[artistColumnKey].length > 15}
									>{row[artistColumnKey]}</span
								>
							</td>
							<td class="font-bold">
								{#if isNotesOnlyItem(row)}
									<!-- skip, this line is for notes, no product code -->
								{:else if isMerchTableItem(row)}
									<em class="unique-item-note">*Merch Table</em>
								{:else}
									{row[productCodeKey]}
								{/if}
							</td>
							<td colspan={isNotesOnlyItem(row) ? 3 : 1}>
								<div class="font-bold" class:notes-only-text={isNotesOnlyItem(row)}>
									{row[productTitleKey]}
								</div>
								{#if !isNotesOnlyItem(row)}
									<div class="text-sm text-gray-600">{row[productTypeKey]}</div>
								{:else if row['Notes']}
									<div class="text-sm text-gray-600">{row['Notes']}</div>
								{/if}
							</td>
							{#if !isNotesOnlyItem(row)}
								<td class="cad-col font-bold">
									<div class="price-cad">{row[priceKey]}</div>
								</td>
								<td class="cart-cell">
									{#if isSoldOutItem(row)}
										<em class="unique-item-note">SOLD OUT</em>
									{:else if canAddToCart(row)}
										<button
											type="button"
											class="add-to-cart-button"
											class:is-added={addedItemIds[rowId(row)]}
											on:click={() => addToCart(row)}
										>
											{addedItemIds[rowId(row)] ? 'Added!' : 'Add to Cart'}
										</button>
									{:else if isMerchTableItem(row)}
										<em class="unique-item-note">*Buy at Merch Table</em>
									{/if}
								</td>
							{/if}
						</tr>
					{/each}
				</tbody>
			{/each}
		</table>

		<!-- mobile layout -->
		<div class="mobile-items">
			{#each artistGroups as group}
				<section class="mobile-artist-group">
					<div class="mobile-artist-divider">
						<ArtistGroupHeading artist={group.artist} details={group.details} />
					</div>
					{#each group.rows as row, i}
						<div
							class="mobile-item"
							class:stripe-alt={i % 2 === 1}
							class:notes-only-row={isNotesOnlyItem(row)}
						>
							{#if isNotesOnlyItem(row)}
								<div class="mobile-center notes-only-mobile">
									<div class="mobile-item-name notes-only-text">{row[productTitleKey]}</div>
									{#if row['Notes']}
										<div class="mobile-item-type">{row['Notes']}</div>
									{/if}
									<div class="mobile-artist">{row[artistColumnKey]}</div>
								</div>
							{:else}
								<div class="mobile-left">
									{#if isMerchTableItem(row)}
										<em class="unique-item-note">*Merch Table</em>
									{:else}
										<div class="mobile-product-code">{row[productCodeKey]}</div>
									{/if}
								</div>
								<div class="mobile-center">
									<div class="mobile-item-name">{row[productTitleKey]}</div>
									<div class="mobile-item-type">{row[productTypeKey]}</div>
									<div class="mobile-artist">{row[artistColumnKey]}</div>
								</div>
								<div class="mobile-prices">
									<div class="mobile-price-cad">{row[priceKey]}</div>
								</div>
								{#if isSoldOutItem(row)}
									<em class="unique-item-note">SOLD OUT</em>
								{:else if canAddToCart(row)}
									<button
										type="button"
										class="add-to-cart-button"
										class:is-added={addedItemIds[rowId(row)]}
										on:click={() => addToCart(row)}
									>
										{addedItemIds[rowId(row)] ? 'Added!' : 'Add to Cart'}
									</button>
								{:else if isMerchTableItem(row)}
									<em class="unique-item-note">*Buy at Merch Table</em>
								{/if}
							{/if}
						</div>
					{/each}
				</section>
			{/each}
		</div>
	{:else}
		<p class="mt-4 text-red-600">No CSV data found.</p>
	{/if}
</div>
