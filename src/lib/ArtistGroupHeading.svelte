<script lang="ts">
	export let artist: string;
	export let details: { label: string; href?: string }[] = [];
</script>

<div class="artist-heading">
	<h2>{artist}</h2>
	{#if details.length}
		<h3>
			{#each details as line, i}
				{#if i > 0}<span class="artist-heading-sep">{' | '}</span>{/if}
				{#if line.href}
					<a href={line.href}>{line.label}</a>
				{:else}
					{line.label}
				{/if}
			{/each}
		</h3>
	{/if}
</div>
