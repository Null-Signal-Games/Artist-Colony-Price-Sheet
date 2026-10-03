<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { browser } from '$app/environment';
	import { afterNavigate } from '$app/navigation';
	import { base } from '$app/paths';
	import { cart, cartCount, cartItemId, parseMoney } from '$lib/cart';
	import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';
	import { shopPromoDetails } from '$lib/artistDetails';
	import { isStaffSession } from '$lib/staff/staffIdentity';
	import { soldOutIds, startSoldOutSync } from '$lib/soldOut';

	export let data: {
		lastUpdated: string;
		csvData: { [key: string]: string }[];
		shopNames: Set<string>;
		artistPromos?: Record<string, { label: string; href?: string }[]>;
	};

	const MERCH_TABLE_FILTER = '__merch_table__';

	let selectedShop = '';
	let searchTerm = '';
	let staffSession = false;

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

	const shopOptions = Array.from(data.shopNames).sort((a, b) => {
		const orderA = shopOrder.get(a) ?? Number.MAX_SAFE_INTEGER;
		const orderB = shopOrder.get(b) ?? Number.MAX_SAFE_INTEGER;
		if (orderA !== orderB) return orderA - orderB;
		return a.localeCompare(b);
	});

	function isMerchTableItem(row: { [key: string]: string }) {
		return (row[productDisplayKey] ?? '').trim().toLowerCase() === 'merch table';
	}

	// === staff only: custom charge order ===
	// useful for discounts and to fix orders
	const CUSTOM_PURCHASE_CODE = 'CUSTOM';
	const customPurchaseRow: { [key: string]: string } = {
		[shopColumnKey]: 'Staff Only',
		[artistColumnKey]: '',
		[productCodeKey]: CUSTOM_PURCHASE_CODE,
		[productTitleKey]: 'Custom Purchase',
		[productTypeKey]: 'Add one for each dollar amount',
		[productDisplayKey]: 'Merch Table',
		Quantity: '',
		[priceKey]: '1.00',
		Notes: ''
	};

	function isCustomPurchaseRow(row: { [key: string]: string }) {
		return row === customPurchaseRow;
	}

	$: showCustomPurchase = (() => {
		if (!staffSession) return false;
		if (selectedShop && selectedShop !== MERCH_TABLE_FILTER) return false;
		const term = searchTerm.trim().toLowerCase();
		if (!term) return true;
		if (isMerchTableQuery(term)) return true;
		return 'custom purchase'.includes(term) || CUSTOM_PURCHASE_CODE.toLowerCase().includes(term);
	})();

	// === end staff only ===

	function isMerchTableQuery(q: string) {
		const normalized = q.toLowerCase().trim();
		if (!normalized) return false;
		const compact = normalized.replace(/[^a-z0-9]/g, '');
		if (compact === 'merch' || compact === 'mt' || compact.includes('merchtable')) return true;
		return /^merch[\s_-]*table(\s+(order|orders|item|items))?$/i.test(normalized);
	}

	function isArtistDisplayItem(row: { [key: string]: string }) {
		return (row[productDisplayKey] ?? '').trim().toLowerCase() === 'artist display';
	}

	function isTitleSoldOut(row: { [key: string]: string }) {
		return /sold out/i.test(row[productTitleKey] ?? '');
	}

	$: soldOutFlags = new Map(
		data.csvData.map((row) => [row, isTitleSoldOut(row) || $soldOutIds.has(rowId(row))])
	);

	function isSoldOutItem(row: { [key: string]: string }) {
		return soldOutFlags.get(row) ?? false;
	}

	function isNotesOnlyItem(row: { [key: string]: string }) {
		const display = (row[productDisplayKey] ?? '').trim().toLowerCase();
		if (display === 'notes') return true;
		return !(row[productCodeKey] ?? '').trim() || !parseMoney(row[priceKey]);
	}

	function canAddToCart(row: { [key: string]: string }) {
    if (isNotesOnlyItem(row) || isSoldOutItem(row)) return false;
		if (isArtistDisplayItem(row)) return true;
		return staffSession && isMerchTableItem(row);
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
				(isMerchTableQuery(term) && isMerchTableItem(row)) ||
				title.includes(term) ||
				(row[productCodeKey] && row[productCodeKey].toLowerCase().includes(term)) ||
				(row[artistColumnKey] && row[artistColumnKey].toLowerCase().includes(term)) ||
				shop.includes(term);

			return (
				matchesSearch &&
				(!selectedShop ||
					(selectedShop === MERCH_TABLE_FILTER
						? isMerchTableItem(row)
						: shopName(row) === selectedShop))
			);
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

	// append to the end of the list
	$: displayGroups = showCustomPurchase
		? [...artistGroups, { artist: 'Staff Only', details: [], rows: [customPurchaseRow] }]
		: artistGroups;

	$: if (browser && displayGroups) {
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
		staffSession = isStaffSession();
		const header = document.querySelector('.fixed-container');
		const columnHeader = document.querySelector('.fixed-header');
		updateArtistStickyState();
		void startSoldOutSync();

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

	afterNavigate(() => {
		staffSession = isStaffSession();
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
		{#if staffSession}
			<a class="title-staff-link" href="{base}/staff">STAFF</a>
		{:else}
			<h1>Artist Colony</h1>
		{/if}
	</div>
	<div class="header-toolbar">
		<div id="filter-fields" class="flex items-center">
			<select bind:value={selectedShop} class="border border-gray-300 p-2">
				<option value="">All Shops</option>
				{#each shopOptions as shop}
					<option value={shop}>{shop}</option>
				{/each}
				<option value={MERCH_TABLE_FILTER}>Merch Table Items</option>
			</select>
			<div class="search-row">
				<input
					type="text"
					placeholder="Search"
					bind:value={searchTerm}
					class="border border-gray-300 p-2"
				/>
			</div>
			{#if selectedShop || searchTerm}
				<button type="button" on:click={clearAll} class="clear-button" aria-label="Clear filters"
					>&times;</button
				>
			{/if}
		</div>
		<a class="header-order-link" href="{base}/order-form" aria-label="View order">
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
	</div>
</div>
<div class="page-container" class:has-order-cta={$cartCount > 0}>
	<div class="site-footer" class:has-order-cta={$cartCount > 0}>
		<div class="footer-notes">
			<p class="footer-notes-text">
				<span class="merch-note">
					*Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch
					Table.
				</span>
				<span class="last-updated-container">
					Last Updated:
					<span class="last-updated-value font-semibold">{data.lastUpdated}</span>
					EDT (UTC-4)
				</span>
			</p>
		</div>
		{#if $cartCount > 0}
			<a class="view-order-button" href="{base}/order-form">View Order ({$cartCount})</a>
		{/if}
	</div>

	{#if displayGroups && displayGroups.length > 0}
		<!-- Fixed header row -->
		<div class="fixed-header">
			<div class="header-cell artist-col">Artist Name</div>
			<div class="header-cell product-code-col">Product Code</div>
			<div class="header-cell product-col">Product</div>
			<div class="header-cell type-col">Type</div>
			<div class="header-cell cad-col">Price (C$)</div>
			<div class="header-cell cart-col"></div>
		</div>

		<!-- Desktop table -->
		<table class="min-w-full">
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
			{#each displayGroups as group}
				<tbody>
					<tr class="artist-divider">
						<td colspan="6">
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
								{:else if isCustomPurchaseRow(row)}
									<div class="merch-table-code">{row[productCodeKey]}</div>
								{:else if isMerchTableItem(row)}
									{#if staffSession && (row[productCodeKey] ?? '').trim()}
										<div class="merch-table-code">{row[productCodeKey]}</div>
									{/if}
									<em class="unique-item-note">*Merch Table</em>
								{:else}
									{row[productCodeKey]}
								{/if}
							</td>
							<td colspan={isNotesOnlyItem(row) ? 4 : 1}>
								<div class="font-bold" class:notes-only-text={isNotesOnlyItem(row)}>
									{row[productTitleKey]}
								</div>
								{#if isNotesOnlyItem(row) && row['Notes']}
									<div class="text-sm text-gray-600">{row['Notes']}</div>
								{/if}
							</td>
							{#if !isNotesOnlyItem(row)}
								<td class="type-col">
									<div class="text-sm text-gray-600">{row[productTypeKey]}</div>
								</td>
								<td class="cad-col font-bold">
									<div class="price-cad">{row[priceKey]}</div>
								</td>
								<td class="cart-cell">
									{#if soldOutFlags.get(row)}
										<span class="sold-out-label">SOLD OUT</span>
									{:else if isMerchTableItem(row)}
										{#if staffSession}
											<button
												type="button"
												class="add-to-cart-button"
												class:is-added={addedItemIds[rowId(row)]}
												on:click={() => addToCart(row)}
											>
												{#if addedItemIds[rowId(row)]}
													Added!
												{:else}
													<span class="add-to-cart-label-full">Add to Cart</span>
													<span class="add-to-cart-label-short">Add</span>
												{/if}
											</button>
										{:else}
											<em class="unique-item-note buy-at-merch-note"
												><span class="buy-merch-line">*Buy at</span
												><span class="buy-merch-line">Merch</span
												><span class="buy-merch-line">Table</span></em
											>
										{/if}
									{:else if isArtistDisplayItem(row)}
										<button
											type="button"
											class="add-to-cart-button"
											class:is-added={addedItemIds[rowId(row)]}
											on:click={() => addToCart(row)}
										>
											{#if addedItemIds[rowId(row)]}
												Added!
											{:else}
												<span class="add-to-cart-label-full">Add to Cart</span>
												<span class="add-to-cart-label-short">Add</span>
											{/if}
										</button>
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
			{#each displayGroups as group}
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
									{#if (row[artistColumnKey] ?? '').trim()}
										<div class="mobile-artist">By {row[artistColumnKey]}</div>
									{/if}
									{#if row['Notes']}
										<div class="mobile-item-type">{row['Notes']}</div>
									{/if}
								</div>
							{:else}
								<div class="mobile-left">
									{#if isCustomPurchaseRow(row)}
										<div class="mobile-product-code merch-table-code">{row[productCodeKey]}</div>
									{:else if isMerchTableItem(row)}
										{#if staffSession && (row[productCodeKey] ?? '').trim()}
											<div class="mobile-product-code merch-table-code">{row[productCodeKey]}</div>
										{/if}
										<em class="unique-item-note">*Merch Table</em>
									{:else}
										<div class="mobile-product-code">{row[productCodeKey]}</div>
									{/if}
								</div>
								<div class="mobile-center">
									<div class="mobile-item-name">{row[productTitleKey]}</div>
									{#if (row[artistColumnKey] ?? '').trim()}
										<div class="mobile-artist">By {row[artistColumnKey]}</div>
									{/if}
									<div class="mobile-item-type">{row[productTypeKey]}</div>
								</div>
								<div class="mobile-prices">
									<div class="mobile-price-cad">{row[priceKey]}</div>
								</div>
								<div class="mobile-action">
									{#if soldOutFlags.get(row)}
										<span class="sold-out-label">SOLD OUT</span>
									{:else if isMerchTableItem(row)}
										{#if staffSession}
											<button
												type="button"
												class="add-to-cart-button"
												class:is-added={addedItemIds[rowId(row)]}
												on:click={() => addToCart(row)}
											>
												{#if addedItemIds[rowId(row)]}
													Added!
												{:else}
													<span class="add-to-cart-label-full">Add to Cart</span>
													<span class="add-to-cart-label-short">Add</span>
												{/if}
											</button>
										{:else}
											<em class="unique-item-note buy-at-merch-note"
												><span class="buy-merch-line">*Buy at</span
												><span class="buy-merch-line">Merch</span
												><span class="buy-merch-line">Table</span></em
											>
										{/if}
									{:else if isArtistDisplayItem(row)}
										<button
											type="button"
											class="add-to-cart-button"
											class:is-added={addedItemIds[rowId(row)]}
											on:click={() => addToCart(row)}
										>
											{#if addedItemIds[rowId(row)]}
												Added!
											{:else}
												<span class="add-to-cart-label-full">Add to Cart</span>
												<span class="add-to-cart-label-short">Add</span>
											{/if}
										</button>
									{/if}
								</div>
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
