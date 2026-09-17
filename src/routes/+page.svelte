<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { browser } from '$app/environment';
  import { cart, cartCount, cartItemId, formatMoney, parseMoney } from '$lib/cart';
  import ArtistGroupHeading from '$lib/ArtistGroupHeading.svelte';
  import { artistDetails } from '$lib/artistDetails';

  export let data: {
    lastUpdated: string;
    csvData: { [key: string]: string }[];
    artistNames: Set<string>;
  };

  type EstimateCurrency = '' | 'USD' | 'EUR' | 'GBP';
  const ESTIMATE_KEY = 'artist-colony-estimate-currency';

  function readEstimateCurrency(): EstimateCurrency {
    if (!browser) return '';
    const stored = localStorage.getItem(ESTIMATE_KEY);
    return stored === 'USD' || stored === 'EUR' || stored === 'GBP' ? stored : '';
  }

  let selectedArtist = '';
  let searchTerm = '';
  let estimateCurrency: EstimateCurrency = readEstimateCurrency();

  $: if (browser) {
    localStorage.setItem(ESTIMATE_KEY, estimateCurrency);
  }

  const headers = Object.keys(data.csvData[0]);
  const artistColumnKey = headers[0];
  const productCodeKey = headers[1];
  const productTitleKey = headers[2];
  const uniqueColumnKey =
    headers.find((key) => key.toLowerCase().replace(/[_\s]/g, '') === 'isunique') ?? 'is_unique';

  function isUniqueItem(row: { [key: string]: string }) {
    return /^(true|1|yes)$/i.test((row[uniqueColumnKey] ?? '').trim());
  }

  $: filteredData = data.csvData.slice(1)
    .filter(row => {
      const term = searchTerm.trim().toLowerCase();
      const matchesSearch =
        !term ||
        (row[productTitleKey] && row[productTitleKey].toLowerCase().includes(term)) ||
        (row[productCodeKey] && row[productCodeKey].toLowerCase().includes(term)) ||
        (row[artistColumnKey] && row[artistColumnKey].toLowerCase().includes(term));

      return matchesSearch && (!selectedArtist || row[artistColumnKey] === selectedArtist);
    })
    .sort((a, b) => {
      // First sort by artist name
      const artistA = a[artistColumnKey].toLowerCase();
      const artistB = b[artistColumnKey].toLowerCase();
      if (artistA !== artistB) {
        return artistA.localeCompare(artistB);
      }
      // If artist names are the same, sort by product code
      const productA = a[headers[1]].toLowerCase();
      const productB = b[headers[1]].toLowerCase();
      return productA.localeCompare(productB);
    });

  $: artistGroups = (() => {
    const groups: {
      artist: string;
      details: { label: string; href?: string }[];
      rows: { [key: string]: string }[];
    }[] = [];

    for (const row of filteredData) {
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
      document.documentElement.style.setProperty('--col-header-height', `${Math.round(colRect.height)}px`);
    }

    document.documentElement.style.setProperty('--artist-sticky-top', `${bottom}px`);
  }

  function updateActiveArtistGroup() {
    if (typeof document === 'undefined') return;

    const stickyTop =
      parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--artist-sticky-top')) || 0;
    const probeY = stickyTop + 1;
    const groups = [...document.querySelectorAll('table tbody, .mobile-artist-group')].filter((group) => {
      const layout = group.closest('table, .mobile-items');
      return !!layout && getComputedStyle(layout).display !== 'none';
    });

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
    selectedArtist = '';
    searchTerm = '';
  }

  function estimatedPrice(row: { [key: string]: string }, currency: Exclude<EstimateCurrency, ''>) {
    if (currency === 'USD') {
      const stored = row[headers[6]];
      if (stored && parseMoney(stored)) return stored.trim();
      return formatMoney(parseMoney(row[headers[5]]) * 0.73, '$');
    }

    if (currency === 'EUR') {
      const stored = row[headers[7]];
      if (stored && parseMoney(stored)) return stored.trim();
      return formatMoney(parseMoney(row[headers[5]]) * 0.62, '€');
    }

    return formatMoney(parseMoney(row[headers[5]]) * 0.54, '£');
  }

  function rowId(row: { [key: string]: string }) {
    return cartItemId({ productCode: row[productCodeKey], title: row[productTitleKey] });
  }

  let addedItemIds: Record<string, boolean> = {};
  const addedTimers = new Map<string, ReturnType<typeof setTimeout>>();

  function flyDotToCart(source: HTMLElement) {
    const cartIcon = document.querySelector('.header-cart-icon');
    if (!(cartIcon instanceof HTMLElement)) return;

    const from = source.getBoundingClientRect();
    const to = cartIcon.getBoundingClientRect();
    const startX = from.left + from.width / 2;
    const startY = from.top + from.height / 2;
    const endX = to.left + to.width / 2;
    const endY = to.top + to.height / 2;

    const dot = document.createElement('span');
    dot.className = 'cart-fly-dot';
    dot.style.left = `${startX}px`;
    dot.style.top = `${startY}px`;
    dot.style.setProperty('--dx', `${endX - startX}px`);
    dot.style.setProperty('--dy', `${endY - startY}px`);
    document.body.appendChild(dot);

    dot.addEventListener(
      'animationend',
      () => {
        dot.remove();
        cartIcon.classList.add('cart-pulse');
        window.setTimeout(() => cartIcon.classList.remove('cart-pulse'), 350);
      },
      { once: true }
    );
  }

  function addToCart(row: { [key: string]: string }, event: MouseEvent) {
    if (isUniqueItem(row)) return;
    const id = rowId(row);
    cart.add({
      id,
      productCode: row[productCodeKey],
      title: row[productTitleKey],
      artist: row[artistColumnKey],
      gbp: row[headers[5]],
      usd: row[headers[6]],
      euro: row[headers[7]]
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
      }, 1400)
    );

    if (event.currentTarget instanceof HTMLElement) {
      flyDotToCart(event.currentTarget);
    }
  }
</script>

<div class="fixed-container">
  <div class="title-bar">
    <h1>Artist Colony</h1>
  </div>
  <div class="estimate-currency">
    <label class="visually-hidden" for="estimate-currency">Est. Price</label>
    <select id="estimate-currency" bind:value={estimateCurrency}>
      <option value="">Est. Price</option>
      <option value="USD">USD</option>
      <option value="EUR">EURO</option>
      <option value="GBP">GBP</option>
    </select>
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
  <div id="filter-fields" class="flex items-center mb-4">
    <select bind:value={selectedArtist} class="p-2 border border-gray-300">
      <option value="">All Artists</option>
      {#each Array.from(data.artistNames).sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase())) as artist}
        <option value={artist}>{artist}</option>
      {/each}
    </select>
    <div class="search-row">
      <input
        type="text"
        placeholder="Search products"
        bind:value={searchTerm}
        class="p-2 border border-gray-300"
      />
    </div>
    {#if selectedArtist || searchTerm}
      <button type="button" on:click={clearAll} class="clear-button" aria-label="Clear filters">&times;</button>
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
        *Buy at Merch Table: Unique Items must be selected and paid for in-person at the Merch Table.
      </p>
      <p class="last-updated-container">
        Last Updated:
        <span class="last-updated-value font-semibold">{data.lastUpdated}</span>
        BST (UTC+1)
      </p>
    </div>
  </div>


  {#if filteredData && filteredData.length > 0}
    <!-- Fixed header row -->
    <div class="fixed-header">
      <div class="header-cell artist-col">{headers[0]}</div>
      <div class="header-cell product-code-col">{headers[1]}</div>
      <div class="header-cell product-col">Product</div>
      <div class="header-cell gbp-col">CAD ($)</div>
      <div class="header-cell usd-col">USD ($)</div>
      <div class="header-cell euro-col">Euro (€)</div>
      <div class="header-cell cart-col"></div>
    </div>

    <!-- Desktop table -->
    <table class="min-w-full">
      <colgroup>
        <col class="col-artist" />
        <col class="col-code" />
        <col class="col-product" />
        <col class="col-cad" />
        <col class="usd-col" />
        <col class="euro-col" />
        <col class="col-cart" />
      </colgroup>
      <thead>
        <tr class="col-sizer">
          <th class="artist-col"></th>
          <th class="product-code-col"></th>
          <th class="product-col"></th>
          <th class="gbp-col"></th>
          <th class="usd-col"></th>
          <th class="euro-col"></th>
          <th class="cart-col"></th>
        </tr>
      </thead>
      {#each artistGroups as group}
        <tbody>
          <tr class="artist-divider">
            <td colspan="7">
              <ArtistGroupHeading artist={group.artist} details={group.details} />
            </td>
          </tr>
          {#each group.rows as row, i}
            <tr class:stripe-alt={i % 2 === 1}>
              <td>
                <span class:small-text={row[artistColumnKey].length > 15}>{row[artistColumnKey]}</span>
              </td>
              <td class="font-bold">
                {#if !isUniqueItem(row)}{row[productCodeKey]}{/if}
              </td>
              <td>
                <div class="font-bold">{row[productTitleKey]}</div>
                <div class="text-sm text-gray-600">{row[headers[3]]}</div>
              </td>
              <td class="font-bold gbp-col">
                <div class="price-cad">{row[headers[5]]}</div>
                {#if estimateCurrency}
                  <div class="price-estimate">({estimatedPrice(row, estimateCurrency)})</div>
                {/if}
              </td>
              <td class="usd-col">{row[headers[6]]}</td>
              <td class="euro-col">{row[headers[7]]}</td>
              <td class="cart-cell">
                {#if isUniqueItem(row)}
                  <em class="unique-item-note">*Buy at Merch Table</em>
                {:else}
                  <button type="button" class="add-to-cart-button" on:click={(event) => addToCart(row, event)}>
                    {addedItemIds[rowId(row)] ? 'Added!' : 'Add to Cart'}
                  </button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      {/each}
    </table>

    <!-- Mobile layout -->
    <div class="mobile-items">
      {#each artistGroups as group}
        <section class="mobile-artist-group">
          <div class="mobile-artist-divider">
            <ArtistGroupHeading artist={group.artist} details={group.details} />
          </div>
          {#each group.rows as row, i}
            <div class="mobile-item" class:stripe-alt={i % 2 === 1}>
              <div class="mobile-left">
                {#if !isUniqueItem(row)}
                  <div class="mobile-product-code">{row[productCodeKey]}</div>
                {/if}
              </div>
              <div class="mobile-center">
                <div class="mobile-item-name">{row[productTitleKey]}</div>
                <div class="mobile-item-type">{row[headers[3]]}</div>
                <div class="mobile-artist">{row[artistColumnKey]}</div>
              </div>
              <div class="mobile-prices">
                <div class="mobile-price-gbp">{row[headers[5]]}</div>
                {#if estimateCurrency}
                  <div class="price-estimate">({estimatedPrice(row, estimateCurrency)})</div>
                {/if}
                <div class="mobile-price-usd">{row[headers[6]]}</div>
                <div class="mobile-price-euro">{row[headers[7]]}</div>
              </div>
              {#if isUniqueItem(row)}
                <em class="unique-item-note">*Buy at Merch Table</em>
              {:else}
                <button type="button" class="add-to-cart-button" on:click={(event) => addToCart(row, event)}>
                  {addedItemIds[rowId(row)] ? 'Added!' : 'Add to Cart'}
                </button>
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
