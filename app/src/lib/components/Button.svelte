<!--
	Unified button primitive. Renders <button> or <a> (when href is set).
	Variants follow the project palette: primary (ink), ghost (neutral),
	danger (muted oxblood), text (no chrome). Sizes: sm | md | lg | icon.

	Distinctive: a Fraunces serif terminator (·) appears on the leading edge
	when `mark` is true — picks up the same italic accent used in the layout
	balance chip (⌇) and the brand "C/M" pill, tying the system together.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLAnchorAttributes, HTMLButtonAttributes } from 'svelte/elements';

	type Variant = 'primary' | 'ghost' | 'danger' | 'text';
	type Size = 'sm' | 'md' | 'lg' | 'icon';

	type CommonProps = {
		variant?: Variant;
		size?: Size;
		mark?: boolean;
		full?: boolean;
		children: Snippet;
	};

	type ButtonProps = CommonProps &
		Omit<HTMLButtonAttributes, 'children'> & {
			href?: undefined;
		};
	type AnchorProps = CommonProps &
		Omit<HTMLAnchorAttributes, 'children'> & {
			href: string;
		};

	let {
		variant = 'primary',
		size = 'md',
		mark = false,
		full = false,
		href,
		children,
		...rest
	}: ButtonProps | AnchorProps = $props();
</script>

{#if href}
	<a
		{href}
		class="btn"
		class:variant-primary={variant === 'primary'}
		class:variant-ghost={variant === 'ghost'}
		class:variant-danger={variant === 'danger'}
		class:variant-text={variant === 'text'}
		class:size-sm={size === 'sm'}
		class:size-md={size === 'md'}
		class:size-lg={size === 'lg'}
		class:size-icon={size === 'icon'}
		class:full
		{...rest as HTMLAnchorAttributes}
	>
		{#if mark}<span class="mark" aria-hidden="true">·</span>{/if}
		{@render children()}
	</a>
{:else}
	<button
		class="btn"
		class:variant-primary={variant === 'primary'}
		class:variant-ghost={variant === 'ghost'}
		class:variant-danger={variant === 'danger'}
		class:variant-text={variant === 'text'}
		class:size-sm={size === 'sm'}
		class:size-md={size === 'md'}
		class:size-lg={size === 'lg'}
		class:size-icon={size === 'icon'}
		class:full
		{...rest as HTMLButtonAttributes}
	>
		{#if mark}<span class="mark" aria-hidden="true">·</span>{/if}
		{@render children()}
	</button>
{/if}

<style>
	.btn {
		--btn-ring: 0 0 0 0 transparent;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		gap: 0.45rem;
		font-family: var(--ui);
		font-weight: 600;
		text-decoration: none;
		border: 1px solid transparent;
		border-radius: 999px;
		cursor: pointer;
		white-space: nowrap;
		transition:
			transform var(--dur-fast) var(--ease-out),
			background-color var(--dur-fast) var(--ease-out),
			border-color var(--dur-fast) var(--ease-out),
			color var(--dur-fast) var(--ease-out),
			box-shadow var(--dur-fast) var(--ease-out);
		box-shadow: var(--btn-ring);
	}
	.btn:focus-visible {
		--btn-ring: 0 0 0 3px var(--accent-soft);
		outline: 1px solid var(--accent);
		outline-offset: 1px;
	}
	.btn[disabled],
	.btn[aria-disabled='true'] {
		opacity: 0.55;
		cursor: not-allowed;
		transform: none !important;
	}
	.btn.full {
		width: 100%;
	}

	/* Sizes */
	.size-sm {
		font-size: 0.78rem;
		padding: 0.4rem 0.85rem;
	}
	.size-md {
		font-size: 0.85rem;
		padding: 0.55rem 1.1rem;
	}
	.size-lg {
		font-size: 0.95rem;
		padding: 0.75rem 1.4rem;
	}
	.size-icon {
		font-size: 1.05rem;
		width: 2.1rem;
		height: 2.1rem;
		padding: 0;
		gap: 0;
	}

	/* Variants */
	.variant-primary {
		background: var(--ink);
		color: var(--bg);
		border-color: var(--ink);
	}
	.variant-primary:not([disabled]):hover {
		background: var(--accent-deep);
		border-color: var(--accent-deep);
		transform: translateY(-1px);
	}
	.variant-primary:not([disabled]):active {
		transform: translateY(0);
	}

	.variant-ghost {
		background: transparent;
		color: var(--ink-2);
		border-color: var(--hairline-2);
	}
	.variant-ghost:not([disabled]):hover {
		color: var(--ink);
		border-color: var(--ink-2);
		background: var(--bg-warm);
	}

	.variant-danger {
		background: transparent;
		color: var(--danger);
		border-color: rgba(183, 50, 35, 0.3);
	}
	.variant-danger:not([disabled]):hover {
		background: rgba(183, 50, 35, 0.06);
		border-color: var(--danger);
	}

	.variant-text {
		background: transparent;
		border: 0;
		color: var(--accent);
		padding-left: 0;
		padding-right: 0;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-thickness: 1px;
		text-decoration-color: rgba(194, 78, 42, 0.4);
		font-weight: 500;
	}
	.variant-text:not([disabled]):hover {
		color: var(--accent-deep);
		text-decoration-color: currentColor;
	}

	/* Distinctive italic Fraunces mark */
	.mark {
		font-family: var(--display);
		font-style: italic;
		font-weight: 400;
		font-size: 1.15em;
		line-height: 0;
		transform: translateY(-0.05em);
		opacity: 0.85;
	}
	.variant-primary .mark {
		color: var(--accent-soft);
	}
	.variant-ghost .mark,
	.variant-danger .mark {
		color: var(--accent);
	}
</style>
