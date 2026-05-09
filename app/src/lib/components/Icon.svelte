<!--
	Inline SVG icon set. Stroke-based, 1.6px round caps — sits cleanly
	next to the Fraunces+Hanken pairing without competing for attention.

	Why inline rather than an icon font / external lib: keeps the bundle
	free of a dependency and lets each glyph carry small editorial
	tweaks (the menu icon's middle bar is intentionally shorter, etc.).
-->
<script lang="ts">
	type Name =
		| 'edit'
		| 'delete'
		| 'close'
		| 'menu'
		| 'download'
		| 'attach'
		| 'chevron-left'
		| 'chevron-right'
		| 'chevron-down';

	let {
		name,
		size = 18,
		stroke = 'currentColor'
	}: { name: Name; size?: number; stroke?: string } = $props();
</script>

<svg
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke={stroke}
	stroke-width="1.6"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	focusable="false"
>
	{#if name === 'edit'}
		<path d="M14.5 4.5l5 5L8 21H3v-5L14.5 4.5z" />
		<path d="M13 6l5 5" />
	{:else if name === 'delete'}
		<path d="M4 7h16" />
		<path d="M9 7V4.5h6V7" />
		<path d="M6 7l1 13a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2l1-13" />
		<path d="M10 11v7M14 11v7" />
	{:else if name === 'close'}
		<path d="M6 6l12 12M18 6L6 18" />
	{:else if name === 'menu'}
		<!-- Editorial twist: middle bar is intentionally shorter, like
		     a hand-set running head. -->
		<path d="M4 7h16" />
		<path d="M4 12h10" />
		<path d="M4 17h16" />
	{:else if name === 'download'}
		<path d="M12 4v12" />
		<path d="M7 11l5 5 5-5" />
		<path d="M5 20h14" />
	{:else if name === 'attach'}
		<path
			d="M21 11.5l-8.49 8.49a5.5 5.5 0 0 1-7.78-7.78l8.49-8.49a3.5 3.5 0 0 1 4.95 4.95L9.69 17.15a1.5 1.5 0 0 1-2.12-2.12l7.78-7.78"
		/>
	{:else if name === 'chevron-left'}
		<path d="M15 6l-6 6 6 6" />
	{:else if name === 'chevron-right'}
		<path d="M9 6l6 6-6 6" />
	{:else if name === 'chevron-down'}
		<path d="M6 9l6 6 6-6" />
	{/if}
</svg>
