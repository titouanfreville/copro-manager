<script lang="ts">
	import '../app.css';
	import { pwaInfo } from 'virtual:pwa-info';
	import { onMount } from 'svelte';

	let { children } = $props();

	onMount(async () => {
		if (pwaInfo) {
			const { useRegisterSW } = await import('virtual:pwa-register/svelte');
			useRegisterSW({ immediate: true });
		}
	});
</script>

<svelte:head>
	{@html pwaInfo ? pwaInfo.webManifest.linkTag : ''}
</svelte:head>

{@render children?.()}
