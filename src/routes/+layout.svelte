<script lang="ts">
	import '../app.css';
	import favicon from '$lib/assets/favicon.svg';
	import { browser } from '$app/environment';
	import { afterNavigate } from '$app/navigation';
	import { onMount } from 'svelte';
	import { syncStaffSessionClass } from '$lib/staff/staffIdentity';

	let { children } = $props();

	onMount(() => {
		syncStaffSessionClass();
	});

	afterNavigate(() => {
		if (browser) syncStaffSessionClass();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		href="https://fonts.googleapis.com/css2?family=Fjalla+One&family=Montserrat:wght@400;600;700&family=Orbitron:wght@700;900&display=swap"
		rel="stylesheet"
	/>
</svelte:head>

{@render children?.()}
