<script lang="ts">
  import { onMount } from 'svelte';
  import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';
  import PrintPageFooter from '$lib/PrintPageFooter.svelte';
  import PrintPageHeader from '$lib/PrintPageHeader.svelte';
  import { shopPromoDetails } from '$lib/artistDetails';

  export let data: {
    lastUpdated: string;
    csvData: { [key: string]: string }[];
    shopNames?: Set<string>;
    artistNames: Set<string>;
    artistPromos?: Record<string, { label: string; href?: string }[]>;
  };

  const shopColumnKey = 'Shop Name';
  const artistColumnKey = 'Artist Name';
  const productCodeKey = 'Product Code';
  const productTitleKey = 'Item Name';
  const productDisplayKey = 'Product Display';
  const productTypeKey = 'Item Type';
  const priceKey = 'Price per unit (C$ CAD)';

  function isMerchTableItem(row: { [key: string]: string }) {
    return (row[productDisplayKey] ?? '').trim().toLowerCase() === 'merch table';
  }

  // for artist shop description only meta lines
  function isNotesOnlyItem(row: { [key: string]: string }) {
    const display = (row[productDisplayKey] ?? '').trim().toLowerCase();
    if (display === 'notes') return true;
    const price = parseFloat(String(row[priceKey] ?? '').replace(/[^0-9.-]/g, ''));
    return !(row[productCodeKey] ?? '').trim() || !Number.isFinite(price) || price <= 0;
  }

  function shopName(row: { [key: string]: string }) {
    return (row[shopColumnKey] || row[artistColumnKey] || '').trim();
  }

  const shopOrder = new Map<string, number>();
  const rowOrder = new Map<{ [key: string]: string }, number>();
  data.csvData.forEach((row, index) => {
    rowOrder.set(row, index);
    const shop = shopName(row);
    if (shop && !shopOrder.has(shop)) shopOrder.set(shop, index);
  });

  const catalogueRows = data.csvData.slice().sort((a, b) => {
    const shopA = shopName(a);
    const shopB = shopName(b);
    const orderA = shopOrder.get(shopA) ?? Number.MAX_SAFE_INTEGER;
    const orderB = shopOrder.get(shopB) ?? Number.MAX_SAFE_INTEGER;
    if (orderA !== orderB) return orderA - orderB;
    return (rowOrder.get(a) ?? 0) - (rowOrder.get(b) ?? 0);
  });

  const artistGroups = (() => {
    const groups: {
      artist: string;
      details: { label: string; href?: string }[];
      rows: { [key: string]: string }[];
    }[] = [];

    for (const row of catalogueRows) {
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

  type PageBlock =
    | { type: 'rule' }
    | { type: 'artist'; groupIndex: number }
    | { type: 'item'; groupIndex: number; rowIndex: number };

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

    const KEEP_TOGETHER_LIMIT = 20;
    const ruleHeight = heightOf(measureRoot.querySelector('[data-print-block="rule"]'));
    const artistHeights = artistGroups.map((_, g) =>
      heightOf(measureRoot.querySelector(`[data-print-block="artist"][data-group-index="${g}"]`))
    );
    const itemHeights = artistGroups.map((group, g) =>
      group.rows.map((_, r) =>
        heightOf(
          measureRoot.querySelector(
            `[data-print-block="item"][data-group-index="${g}"][data-row-index="${r}"]`
          )
        )
      )
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

    const groupContentHeight = (groupIndex: number) =>
      (artistHeights[groupIndex] ?? 0) +
      itemHeights[groupIndex].reduce((total, height) => total + height, 0);

    for (let g = 0; g < artistGroups.length; g++) {
      const group = artistGroups[g];
      const keepTogether = group.rows.length < KEEP_TOGETHER_LIMIT;
      const contentHeight = groupContentHeight(g);
      const leadHeight = current.length > 0 ? ruleHeight : 0;
      const totalNeeded = leadHeight + contentHeight;

      if (keepTogether && contentHeight <= available) {
        if (current.length > 0 && used + totalNeeded > available) {
          pushPage();
        }

        if (current.length > 0) {
          add({ type: 'rule' }, ruleHeight);
        }
        add({ type: 'artist', groupIndex: g }, artistHeights[g] ?? 0);
        artistOnPage = g;

        for (let r = 0; r < group.rows.length; r++) {
          add({ type: 'item', groupIndex: g, rowIndex: r }, itemHeights[g][r]);
        }
        continue;
      }

      ensureArtist(g);

      for (let r = 0; r < group.rows.length; r++) {
        const itemHeight = itemHeights[g][r];

        if (itemHeight > 0 && used + itemHeight > available && current.length) {
          pushPage();
          ensureArtist(g);
        }

        add({ type: 'item', groupIndex: g, rowIndex: r }, itemHeight);
      }
    }

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
    <p class="print-toolbar-hint">Letter pages · Gray gaps are page breaks</p>
    <button type="button" class="print-pdf-button" on:click={printCatalogue}
      >Print / Save PDF</button
    >
  </header>

  <div class="print-measure" bind:this={measureRoot} aria-hidden="true">
    <section class="print-page">
      <PrintPageHeader />
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
                  <th>Price (C$)</th>
                </tr>
              </tbody>
              <tbody>
                {#each group.rows as row, r}
                  <tr data-print-block="item" data-group-index={g} data-row-index={r}>
                    <td class="print-code-cell">
                      {#if isNotesOnlyItem(row)}{:else if isMerchTableItem(row)}
                        <em class="print-merch-note">*Merch Table</em>
                      {:else}
                        {row[productCodeKey]}
                      {/if}
                    </td>
                    <td class="print-product-cell" colspan={isNotesOnlyItem(row) ? 3 : 1}>
                      <span class="print-product-name">{row[productTitleKey]}</span>
                    </td>
                    {#if !isNotesOnlyItem(row)}
                      <td class="print-type-cell">{row[productTypeKey]}</td>
                      <td class="print-cad-cell">{row[priceKey]}</td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            {/each}
          </table>
        {/if}
      </div>
      <PrintPageFooter pageNumber={1} />
    </section>
  </div>

  <div class="print-sheet-wrap">
    {#if pages.length > 0}
      {#each pages as page, i}
        {#if i > 0}
          <div class="print-break-marker">Page break · Page {i + 1}</div>
        {/if}
        <section class="print-page">
          <PrintPageHeader />
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
                      <th>Price (C$)</th>
                    </tr>
                  </tbody>
                {:else if block.type === 'item'}
                  {@const row = artistGroups[block.groupIndex].rows[block.rowIndex]}
                  <tbody>
                    <tr>
                      <td class="print-code-cell">
                        {#if isNotesOnlyItem(row)}
                          <!-- notes-only -->
                        {:else if isMerchTableItem(row)}
                          <em class="print-merch-note">*Merch Table</em>
                        {:else}
                          {row[productCodeKey]}
                        {/if}
                      </td>
                      <td class="print-product-cell" colspan={isNotesOnlyItem(row) ? 3 : 1}>
                        <span class="print-product-name">{row[productTitleKey]}</span>
                      </td>
                      {#if !isNotesOnlyItem(row)}
                        <td class="print-type-cell">{row[productTypeKey]}</td>
                        <td class="print-cad-cell">{row[priceKey]}</td>
                      {/if}
                    </tr>
                  </tbody>
                {/if}
              {/each}
            </table>
          </div>
          <PrintPageFooter pageNumber={i + 1} />
        </section>
      {/each}
    {:else if artistGroups.length > 0}
      <section class="print-page print-page-fallback">
        <PrintPageHeader />
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
                  <th>Price (C$)</th>
                </tr>
                {#each group.rows as row}
                  <tr>
                    <td class="print-code-cell">
                      {#if isNotesOnlyItem(row)}
                        <!-- notes-only -->
                      {:else if isMerchTableItem(row)}
                        <em class="print-merch-note">*Merch Table</em>
                      {:else}
                        {row[productCodeKey]}
                      {/if}
                    </td>
                    <td class="print-product-cell" colspan={isNotesOnlyItem(row) ? 3 : 1}>
                      <span class="print-product-name">{row[productTitleKey]}</span>
                    </td>
                    {#if !isNotesOnlyItem(row)}
                      <td class="print-type-cell">{row[productTypeKey]}</td>
                      <td class="print-cad-cell">{row[priceKey]}</td>
                    {/if}
                  </tr>
                {/each}
              </tbody>
            {/each}
          </table>
        </div>
        <PrintPageFooter pageNumber={1} />
      </section>
    {:else}
      <p>No CSV data found.</p>
    {/if}
  </div>
</div>
