<!--
	Floating action button. Same visual language as the primary <Button>
	but anchored bottom-right with a more pronounced shadow so it sits
	above content on mobile. The italic Fraunces "+" preserves the
	editorial flourish from the brand mark.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Props = Omit<HTMLButtonAttributes, 'children'> & {
		children: Snippet;
	};

	let { children, ...rest }: Props = $props();
</script>

<button class="fab" type="button" {...rest}>
	<span class="plus" aria-hidden="true">+</span>
	{@render children()}
</button>

<style>
	.fab {
		position: fixed;
		bottom: 1.4rem;
		right: 1.4rem;
		z-index: 30;
		display: inline-flex;
		align-items: center;
		gap: 0.55rem;
		padding: 0.85rem 1.35rem 0.85rem 1.05rem;
		font-family: var(--ui);
		font-size: 0.85rem;
		font-weight: 600;
		color: var(--bg);
		background: var(--ink);
		border: 0;
		border-radius: 999px;
		cursor: pointer;
		box-shadow:
			0 16px 32px rgba(20, 16, 12, 0.18),
			0 4px 10px rgba(20, 16, 12, 0.08);
		transition:
			transform var(--dur-fast) var(--ease-out),
			background-color var(--dur-fast) var(--ease-out),
			box-shadow var(--dur-fast) var(--ease-out);
	}
	.fab:hover {
		background: var(--accent-deep);
		transform: translateY(-2px);
		box-shadow:
			0 20px 38px rgba(143, 58, 31, 0.28),
			0 6px 14px rgba(20, 16, 12, 0.1);
	}
	.fab:active {
		transform: translateY(0);
	}
	.fab:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 3px;
	}
	.plus {
		font-family: var(--display);
		font-style: italic;
		font-size: 1.35rem;
		line-height: 1;
		color: var(--accent-soft);
		transform: translateY(-0.04em);
	}

	@media (max-width: 480px) {
		.fab {
			bottom: 1rem;
			right: 1rem;
			padding: 0.75rem 1.1rem 0.75rem 0.9rem;
		}
	}
</style>
