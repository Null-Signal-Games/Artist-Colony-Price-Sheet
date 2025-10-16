<script lang="ts">
  export let data: {
    lastUpdated: string;
    csvData: { [key: string]: string }[];
    artistNames: Set<string>;
  };

  let selectedArtist = '';
  let searchTerm = '';

  const headers = Object.keys(data.csvData[0]);
  const artistColumnKey = headers[0];
  const productColumnKey = headers[2];

  $: filteredData = data.csvData.slice(1).filter(row =>
    (!selectedArtist || row[artistColumnKey] === selectedArtist) &&
    (!searchTerm || row[productColumnKey].toLowerCase().includes(searchTerm.toLowerCase()))
  );

  function clearAll() {
    selectedArtist = '';
    searchTerm = '';
  }
</script>

<div class="fixed-container">
  <h1 class="text-3xl font-bold mb-4">Artist Colony Price Sheet</h1>
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
<div class="page-container">
  <p class="last-updated-container mb-4 text-gray-600">
    Last Updated:
    <span class="last-updated-value font-semibold">{data.lastUpdated}</span>
    BST (UTC+1)
  </p>


  {#if filteredData && filteredData.length > 0}
    <!-- Desktop table -->
    <table class="min-w-full border-collapse border border-gray-300">
      <thead>
        <tr class="bg-gray-200">
          <th class="border border-gray-300 px-4 py-2">{headers[0]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[1]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[2]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[3]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[5]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[6]}</th>
          <th class="border border-gray-300 px-4 py-2">{headers[7]}</th>
        </tr>
      </thead>
      <tbody>
        {#each filteredData as row}
          <tr>
            <td class="border border-gray-300 px-4 py-2">{row[headers[0]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[1]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[2]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[3]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[5]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[6]]}</td>
            <td class="border border-gray-300 px-4 py-2">{row[headers[7]]}</td>
          </tr>
        {/each}
      </tbody>
    </table>

    <!-- Mobile layout -->
    <div class="mobile-items">
      {#each filteredData as row}
        <div class="mobile-item">
          <div class="mobile-left">
            <div class="mobile-product-code">{row[headers[1]]}</div>
          </div>
          <div class="mobile-center">
            <div class="mobile-item-name">{row[productColumnKey]}</div>
            <div class="mobile-item-type">{row[headers[3]]}</div>
            <div class="mobile-artist">{row[artistColumnKey]}</div>
          </div>
          <div class="mobile-right">
            <div class="mobile-price-gbp">{row[headers[5]]}</div>
            <div class="mobile-price-usd">{row[headers[6]]}</div>
            <div class="mobile-price-euro">{row[headers[7]]}</div>
          </div>
        </div>
      {/each}
    </div>
  {:else}
    <p class="mt-4 text-red-600">No CSV data found.</p>
  {/if}
</div>
