<script lang="ts">
	import { onMount } from 'svelte';
	import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';
	import { artistDetails } from '$lib/artistDetails';

	export let data: {
		lastUpdated: string;
		csvData: { [key: string]: string }[];
		artistNames: Set<string>;
	};

	const headers = Object.keys(data.csvData[0] ?? {});
	const artistColumnKey = headers[0];
	const productCodeKey = headers[1];
	const productTitleKey = headers[2];
	const productTypeKey = headers[3];
	const uniqueColumnKey =
		headers.find((key) => key.toLowerCase().replace(/[_\s]/g, '') === 'isunique') ?? 'is_unique';

	function isUniqueItem(row: { [key: string]: string }) {
		return /^(true|1|yes)$/i.test((row[uniqueColumnKey] ?? '').trim());
	}

	const catalogueRows = data.csvData.slice(1).sort((a, b) => {
		const artistA = (a[artistColumnKey] ?? '').toLowerCase();
		const artistB = (b[artistColumnKey] ?? '').toLowerCase();
		if (artistA !== artistB) {
			return artistA.localeCompare(artistB);
		}
		return (a[productCodeKey] ?? '').toLowerCase().localeCompare((b[productCodeKey] ?? '').toLowerCase());
	});

	const artistGroups = (() => {
		const groups: {
			artist: string;
			details: { label: string; href?: string }[];
			rows: { [key: string]: string }[];
		}[] = [];

		for (const row of catalogueRows) {
			const artist = row[artistColumnKey];
			const last = groups[groups.length - 1];

			if (!last || last.artist !== artist) {
				groups.push({ artist, details: artistDetails(artist), rows: [row] });
			} else {
				last.rows.push(row);
			}
		}

		return groups;
	})();

	type PageBlock =
		| { type: 'rule' }
		| { type: 'artist'; groupIndex: number }
		| { type: 'item'; groupIndex: number; rowIndex: number }
		| { type: 'footer' };

	type PrintPage = { blocks: PageBlock[] };

	let measureRoot: HTMLElement;
	let pages: PrintPage[] = [];

	function heightOf(el: Element | null) {
		return el instanceof HTMLElement ? el.offsetHeight : 0;
	}

	function paginate() {
		if (!measureRoot || artistGroups.length === 0) return;

		const body = measureRoot.querySelector('.print-page-body');
		const available = body instanceof HTMLElement ? Math.max(0, body.clientHeight - 12) : 0;
		if (available <= 0) return;

		const ruleHeight = heightOf(measureRoot.querySelector('[data-print-block="rule"]'));
		const footerHeight = heightOf(measureRoot.querySelector('[data-print-block="footer"]'));
		const artistHeights = artistGroups.map((_, g) =>
			heightOf(measureRoot.querySelector(`[data-print-block="artist"][data-group-index="${g}"]`))
		);

		const nextPages: PrintPage[] = [];
		let current: PageBlock[] = [];
		let used = 0;
		let artistOnPage: number | null = null;

		const pushPage = () => {
			if (current.length) nextPages.push({ blocks: current });
			current = [];
			used = 0;
			artistOnPage = null;
		};

		const add = (block: PageBlock, height: number) => {
			if (height > 0 && used + height > available && current.length) {
				pushPage();
			}
			current.push(block);
			used += height;
		};

		const ensureArtist = (groupIndex: number) => {
			if (artistOnPage === groupIndex) return;

			const startHeight = artistHeights[groupIndex] ?? 0;
			const withRule = current.length > 0 ? ruleHeight + startHeight : startHeight;

			if (withRule > 0 && used + withRule > available && current.length) {
				pushPage();
			}

			if (current.length > 0) {
				add({ type: 'rule' }, ruleHeight);
			}

			add({ type: 'artist', groupIndex }, startHeight);
			artistOnPage = groupIndex;
		};

		for (let g = 0; g < artistGroups.length; g++) {
			const group = artistGroups[g];
			ensureArtist(g);

			for (let r = 0; r < group.rows.length; r++) {
				const itemHeight = heightOf(
					measureRoot.querySelector(
						`[data-print-block="item"][data-group-index="${g}"][data-row-index="${r}"]`
					)
				);

				if (itemHeight > 0 && used + itemHeight > available && current.length) {
					pushPage();
					ensureArtist(g);
				}

				add({ type: 'item', groupIndex: g, rowIndex: r }, itemHeight);
			}
		}

		if (footerHeight > 0 && used + footerHeight > available && current.length) {
			pushPage();
		}
		add({ type: 'footer' }, footerHeight);
		pushPage();

		pages = nextPages;
	}

	onMount(() => {
		const run = () => paginate();
		run();
		document.fonts?.ready?.then(run);
		window.addEventListener('resize', run);
		return () => window.removeEventListener('resize', run);
	});

	function printCatalogue() {
		window.print();
	}
</script>

<svelte:head>
	<title>Artist Colony 2026 Price List</title>
</svelte:head>

<div class="print-preview-page">
	<header class="print-toolbar">
		<a class="print-back-link" href="/">← Price sheet</a>
		<p class="print-toolbar-hint">A4 pages · Gray gaps are page breaks</p>
		<button type="button" class="print-pdf-button" on:click={printCatalogue}>Print / Save PDF</button>
	</header>

	<div class="print-measure" bind:this={measureRoot} aria-hidden="true">
		<section class="print-page">
			<header class="print-page-header">
				<h1>Artist Colony 2026 Price List</h1>
				<p class="print-page-number">Page 1</p>
			</header>
			<div class="print-page-body">
				{#if artistGroups.length > 0}
					<table class="print-catalogue">
						<colgroup>
							<col class="print-col-code" />
							<col class="print-col-product" />
							<col class="print-col-type" />
							<col class="print-col-cad" />
						</colgroup>
						<tbody>
							<tr class="print-group-rule" data-print-block="rule">
								<td colspan="4">
									<div class="print-artist-rule" aria-hidden="true"></div>
								</td>
							</tr>
						</tbody>
						{#each artistGroups as group, g}
							<tbody data-print-block="artist" data-group-index={g}>
								<tr class="print-artist-heading-row">
									<td colspan="4">
										<ArtistGroupHeading artist={group.artist} details={group.details} />
									</td>
								</tr>
								<tr class="print-col-headers">
									<th>Code</th>
									<th>Product</th>
									<th>Type</th>
									<th>CAD ($)</th>
								</tr>
							</tbody>
							<tbody>
								{#each group.rows as row, r}
									<tr data-print-block="item" data-group-index={g} data-row-index={r}>
										<td class="print-code-cell">
											{#if isUniqueItem(row)}
												<em class="print-merch-note">*Merch Table</em>
											{:else}
												{row[productCodeKey]}
											{/if}
										</td>
										<td class="print-product-cell">
											<span class="print-product-name">{row[productTitleKey]}</span>
										</td>
										<td class="print-type-cell">{row[productTypeKey]}</td>
										<td class="print-cad-cell">{row[headers[5]]}</td>
									</tr>
								{/each}
							</tbody>
						{/each}
					</table>
					<p class="print-merch-footer" data-print-block="footer">
						*Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch Table.
					</p>
				{/if}
			</div>
		</section>
	</div>

	<div class="print-sheet-wrap">
		{#if pages.length > 0}
			{#each pages as page, i}
				{#if i > 0}
					<div class="print-break-marker">Page break · Page {i + 1}</div>
				{/if}
				<section class="print-page">
					<header class="print-page-header">
						<h1>Artist Colony 2026 Price List</h1>
						<p class="print-page-number">Page {i + 1}</p>
					</header>
					<div class="print-page-body">
						<table class="print-catalogue">
							<colgroup>
								<col class="print-col-code" />
								<col class="print-col-product" />
								<col class="print-col-type" />
								<col class="print-col-cad" />
							</colgroup>
							{#each page.blocks as block}
								{#if block.type === 'rule'}
									<tbody>
										<tr class="print-group-rule">
											<td colspan="4">
												<div class="print-artist-rule" aria-hidden="true"></div>
											</td>
										</tr>
									</tbody>
								{:else if block.type === 'artist'}
									<tbody>
										<tr class="print-artist-heading-row">
											<td colspan="4">
												<ArtistGroupHeading
													artist={artistGroups[block.groupIndex].artist}
													details={artistGroups[block.groupIndex].details}
												/>
											</td>
										</tr>
										<tr class="print-col-headers">
											<th>Code</th>
											<th>Product</th>
											<th>Type</th>
											<th>CAD ($)</th>
										</tr>
									</tbody>
								{:else if block.type === 'item'}
									{@const row = artistGroups[block.groupIndex].rows[block.rowIndex]}
									<tbody>
										<tr>
											<td class="print-code-cell">
												{#if isUniqueItem(row)}
													<em class="print-merch-note">*Merch Table</em>
												{:else}
													{row[productCodeKey]}
												{/if}
											</td>
											<td class="print-product-cell">
												<span class="print-product-name">{row[productTitleKey]}</span>
											</td>
											<td class="print-type-cell">{row[productTypeKey]}</td>
											<td class="print-cad-cell">{row[headers[5]]}</td>
										</tr>
									</tbody>
								{:else if block.type === 'footer'}
									<!-- footer is outside the table -->
								{/if}
							{/each}
						</table>
						{#if page.blocks.some((block) => block.type === 'footer')}
							<p class="print-merch-footer">
								*Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch
								Table.
							</p>
						{/if}
					</div>
				</section>
			{/each}
		{:else if artistGroups.length > 0}
			<section class="print-page print-page-fallback">
				<header class="print-page-header">
					<h1>Artist Colony 2026 Price List</h1>
					<p class="print-page-number">Page 1</p>
				</header>
				<div class="print-page-body">
					<table class="print-catalogue">
						<colgroup>
							<col class="print-col-code" />
							<col class="print-col-product" />
							<col class="print-col-type" />
							<col class="print-col-cad" />
						</colgroup>
						{#each artistGroups as group, g}
							<tbody>
								{#if g > 0}
									<tr class="print-group-rule">
										<td colspan="4">
											<div class="print-artist-rule" aria-hidden="true"></div>
										</td>
									</tr>
								{/if}
								<tr class="print-artist-heading-row">
									<td colspan="4">
										<ArtistGroupHeading artist={group.artist} details={group.details} />
									</td>
								</tr>
								<tr class="print-col-headers">
									<th>Code</th>
									<th>Product</th>
									<th>Type</th>
									<th>CAD ($)</th>
								</tr>
								{#each group.rows as row}
									<tr>
										<td class="print-code-cell">
											{#if isUniqueItem(row)}
												<em class="print-merch-note">*Merch Table</em>
											{:else}
												{row[productCodeKey]}
											{/if}
										</td>
										<td class="print-product-cell">
											<span class="print-product-name">{row[productTitleKey]}</span>
										</td>
										<td class="print-type-cell">{row[productTypeKey]}</td>
										<td class="print-cad-cell">{row[headers[5]]}</td>
									</tr>
								{/each}
							</tbody>
						{/each}
					</table>
					<p class="print-merch-footer">
						*Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch Table.
					</p>
				</div>
			</section>
		{:else}
			<p>No CSV data found.</p>
		{/if}
	</div>
</div>
