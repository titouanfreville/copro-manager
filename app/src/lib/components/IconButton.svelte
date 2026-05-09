<!--
	Icon-only button. 44px touch target by default (iOS/Android minimum
	for ergonomic taps). Designed to live inside cards, modal headers,
	or as the mobile burger trigger — anywhere a label would be noise.

	Renders <a> when `href` is provided, <button> otherwise. Always
	require an explicit `aria-label` so screen readers announce the
	action even though the visible glyph is silent.
-->
<script lang="ts">
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';
	import Icon from './Icon.svelte';

	type IconName =
		| 'edit'
		| 'delete'
		| 'close'
		| 'menu'
		| 'download'
		| 'attach'
		| 'chevron-left'
		| 'chevron-right'
		| 'chevron-down';
	type Variant = 'ghost' | 'danger' | 'text';
	type Size = 'sm' | 'md';

	type CommonProps = {
		icon: IconName;
		'aria-label': string;
		variant?: Variant;
		size?: Size;
		iconSize?: number;
	};

	type ButtonProps = CommonProps &
		Omit<HTMLButtonAttributes, 'children' | 'aria-label'> & { href?: undefined };
	type AnchorProps = CommonProps &
		Omit<HTMLAnchorAttributes, 'children' | 'aria-label'> & { href: string };

	let {
		icon,
		variant = 'ghost',
		size = 'md',
		iconSize,
		href,
		...rest
	}: ButtonProps | AnchorProps = $props();

	let resolvedIconSize = $derived(iconSize ?? (size === 'sm' ? 16 : 18));
</script>

{#if href}
	<a
		{href}
		class="icon-btn"
		class:variant-ghost={variant === 'ghost'}
		class:variant-danger={variant === 'danger'}
		class:variant-text={variant === 'text'}
		class:size-sm={size === 'sm'}
		class:size-md={size === 'md'}
		{...rest as HTMLAnchorAttributes}
	>
		<Icon name={icon} size={resolvedIconSize} />
	</a>
{:else}
	<button
		type="button"
		class="icon-btn"
		class:variant-ghost={variant === 'ghost'}
		class:variant-danger={variant === 'danger'}
		class:variant-text={variant === 'text'}
		class:size-sm={size === 'sm'}
		class:size-md={size === 'md'}
		{...rest as HTMLButtonAttributes}
	>
		<Icon name={icon} size={resolvedIconSize} />
	</button>
{/if}

<style>
	.icon-btn {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		border: 1px solid transparent;
		border-radius: 999px;
		background: transparent;
		color: var(--ink-2);
		cursor: pointer;
		padding: 0;
		text-decoration: none;
		transition:
			background-color var(--dur-fast) var(--ease-out),
			border-color var(--dur-fast) var(--ease-out),
			color var(--dur-fast) var(--ease-out),
			transform var(--dur-fast) var(--ease-out);
	}
	.icon-btn:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.icon-btn[disabled],
	.icon-btn[aria-disabled='true'] {
		opacity: 0.45;
		cursor: not-allowed;
	}

	/* Sizes — md = 44px (mobile-comfy), sm = 36px (in-card secondary). */
	.size-md {
		width: 2.75rem;
		height: 2.75rem;
	}
	.size-sm {
		width: 2.25rem;
		height: 2.25rem;
	}

	.variant-ghost {
		border-color: var(--hairline-2);
	}
	.variant-ghost:not([disabled]):hover {
		background: var(--bg-warm);
		border-color: var(--ink-3);
		color: var(--ink);
	}
	.variant-ghost:not([disabled]):active {
		transform: scale(0.96);
	}

	.variant-danger {
		border-color: rgba(183, 50, 35, 0.25);
		color: var(--danger);
	}
	.variant-danger:not([disabled]):hover {
		background: rgba(183, 50, 35, 0.08);
		border-color: var(--danger);
	}

	.variant-text {
		border-color: transparent;
		color: var(--ink-3);
	}
	.variant-text:not([disabled]):hover {
		color: var(--ink);
		background: var(--bg-warm);
	}
</style>
