<script lang="ts">
	export let data;
</script>

<h1>Artist Colony Price Sheet</h1>
<p class="last-updated-container">
	Last Updated:
	<span class="last-updated-value">{data.lastUpdated}</span>
	BST (UTC+1)
</p>

{#if data.csvData && data.csvData.length > 0}
	<table>
		<thead>
			<tr>
				{#each Object.keys(data.csvData[0]) as header}
					<th>{header}</th>
				{/each}
			</tr>
		</thead>
		<tbody>
			{#each data.csvData as row}
				<tr>
					{#each Object.values(row) as cell}
						<td>{cell}</td>
					{/each}
				</tr>
			{/each}
		</tbody>
	</table>
{:else}
	<p>No CSV data found.</p>
{/if}
