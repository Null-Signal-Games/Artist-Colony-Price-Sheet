<script lang="ts">
  import { cart, cartCount, cartItemId } from '$lib/cart';

  export let data: {
    lastUpdated: string;
    csvData: { [key: string]: string }[];
    artistNames: Set<string>;
  };

  let selectedArtist = '';
  let searchTerm = '';

  const headers = Object.keys(data.csvData[0]);
  const artistColumnKey = headers[0];
  const productCodeKey = headers[1];
  const productTitleKey = headers[2];

  $: filteredData = data.csvData.slice(1)
    .filter(row =>
      (
        (!searchTerm || 
        (row[productTitleKey] && row[productTitleKey].toLowerCase().includes(searchTerm.toLowerCase())) || 
        (row[productCodeKey] && row[productCodeKey].toLowerCase().includes(searchTerm.toLowerCase())))
        && (!selectedArtist || row[artistColumnKey] === selectedArtist) // Filter by selected artist
      )
    )
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

  function clearAll() {
    selectedArtist = '';
    searchTerm = '';
  }

  function addToCart(row: { [key: string]: string }) {
    cart.add({
      id: cartItemId({ productCode: row[productCodeKey], title: row[productTitleKey] }),
      productCode: row[productCodeKey],
      title: row[productTitleKey],
      artist: row[artistColumnKey],
      gbp: row[headers[5]],
      usd: row[headers[6]],
      euro: row[headers[7]]
    });
  }
</script>

<div class="fixed-container">
  <div class="title-bar">
    <h1>Artist Colony Order Form</h1>
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
      {#if selectedArtist || searchTerm}
        <button on:click={clearAll} class="clear-button">&times;</button>
      {/if}
    </div>
  </div>
</div>
<div class="page-container" class:has-order-cta={$cartCount > 0}>
  <div class="site-footer" class:has-order-cta={$cartCount > 0}>
    {#if $cartCount > 0}
      <a class="view-order-button" href="/order-form">View Order ({$cartCount})</a>
    {/if}
    <p class="last-updated-container">
      Last Updated:
      <span class="last-updated-value font-semibold">{data.lastUpdated}</span>
      BST (UTC+1)
    </p>
  </div>


  {#if filteredData && filteredData.length > 0}
    <!-- Fixed header row -->
    <div class="fixed-header">
      <div class="header-cell artist-col">{headers[0]}</div>
      <div class="header-cell product-code-col">{headers[1]}</div>
      <div class="header-cell product-col">Product</div>
      <div class="header-cell gbp-col">GBP (£)</div>
      <div class="header-cell usd-col">USD ($)</div>
      <div class="header-cell euro-col">Euro (€)</div>
      <div class="header-cell cart-col"></div>
    </div>

    <!-- Desktop table -->
    <table class="min-w-full border-collapse border border-gray-300">
      <tbody>
        {#each filteredData as row}
          <tr>
            <td class="border border-gray-300 px-4 py-2">
              <span class:small-text={row[artistColumnKey].length > 15}>{row[artistColumnKey]}</span>
            </td>
            <td class="border border-gray-300 px-4 py-2 font-bold">{row[productCodeKey]}</td>
            <td class="border border-gray-300 px-4 py-2">
              <div class="font-bold">{row[productTitleKey]}</div>
              <div class="text-sm text-gray-600">{row[headers[3]]}</div>
            </td>
            <td class="border border-gray-300 px-4 py-2 font-bold">{row[headers[5]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[6]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[7]]}</td>
            <td class="border border-gray-300 px-4 py-2 cart-cell">
              <button type="button" class="add-to-cart-button" on:click={() => addToCart(row)}>
                Add to Cart
              </button>
            </td>
          </tr>
        {/each}
      </tbody>
    </table>

    <!-- Mobile layout -->
    <div class="mobile-items">
      {#each filteredData as row}
        <div class="mobile-item">
          <div class="mobile-left">
            <div class="mobile-product-code">{row[productCodeKey]}</div>
          </div>
          <div class="mobile-center">
            <div class="mobile-item-name">{row[productTitleKey]}</div>
            <div class="mobile-item-type">{row[headers[3]]}</div>
            <div class="mobile-artist">{row[artistColumnKey]}</div>
          </div>
          <div class="mobile-prices">
            <div class="mobile-price-gbp">{row[headers[5]]}</div>
            <div class="mobile-price-usd">{row[headers[6]]}</div>
            <div class="mobile-price-euro">{row[headers[7]]}</div>
          </div>
          <button type="button" class="add-to-cart-button" on:click={() => addToCart(row)}>
            Add to Cart
          </button>
        </div>
      {/each}
    </div>
  {:else}
    <p class="mt-4 text-red-600">No CSV data found.</p>
  {/if}
</div>
