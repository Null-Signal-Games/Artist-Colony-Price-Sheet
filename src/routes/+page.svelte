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
</script>

<h1 class="text-3xl font-bold mb-4">Artist Colony Price Sheet</h1>
<p class="last-updated-container mb-4 text-gray-600">
  Last Updated:
  <span class="last-updated-value font-semibold">{data.lastUpdated}</span>
  BST (UTC+1)
</p>

<div id="filter-fields" class="flex items-center mb-4">
  <select bind:value={selectedArtist} class="m-2 p-2 border border-gray-300">
    <option value="">All Artists</option>
    {#each Array.from(data.artistNames).sort((a, b) => a.toLowerCase().localeCompare(b.toLowerCase())) as artist}
      <option value={artist}>{artist}</option>
    {/each}
  </select>
  <input
    type="text"
    placeholder="Search products"
    bind:value={searchTerm}
    class="m-2 p-2 border border-gray-300"
  />
</div>

{#if filteredData && filteredData.length > 0}
  <table class="min-w-full border-collapse border border-gray-300">
    <thead>
      <tr class="bg-gray-200">
        {#each headers as header}
          <th class="border border-gray-300 px-4 py-2">{header}</th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each filteredData as row}
        <tr>
          {#each Object.values(row) as cell}
            <td class="border border-gray-300 px-4 py-2">{cell}</td>
          {/each}
        </tr>
      {/each}
    </tbody>
  </table>
{:else}
  <p class="mt-4 text-red-600">No CSV data found.</p>
{/if}
