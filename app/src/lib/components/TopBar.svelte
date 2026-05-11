<!--
	Page header used by every signed-in route. Brand on the left, nav in
	the middle on desktop, user block on the right.

	Mobile (<= 720px): nav and user block collapse into a burger sheet
	that slides down. Brand stays visible, the burger tap target is 44px
	for comfortable thumb reach. Sheet dismisses via backdrop, Esc, or
	tapping any item.

	The active route gets a Fraunces italic underscore beneath it (or a
	leading mark in the mobile sheet) — same editorial flourish reused
	throughout the system, so the components feel of a piece.
-->
<script lang="ts">
	import { page } from '$app/stores';
	import { authState, logout } from '$lib/auth';
	import AlertsBell from './AlertsBell.svelte';
	import IconButton from './IconButton.svelte';

	type NavItem = { href: string; label: string };

	let { items = defaultItems() }: { items?: NavItem[] } = $props();

	function defaultItems(): NavItem[] {
		return [
			{ href: '/expenses', label: 'Dépenses' },
			{ href: '/templates', label: 'Modèles' },
			{ href: '/meters', label: 'Compteurs' },
			{ href: '/contracts', label: 'Contrats' },
			{ href: '/documents', label: 'Documents' },
			{ href: '/categories', label: 'Catégories' }
		];
	}

	let pathname = $derived($page.url.pathname);
	function isActive(href: string): boolean {
		if (href === '/') return pathname === '/';
		return pathname === href || pathname.startsWith(href + '/');
	}

	let menuOpen = $state(false);
	function openMenu() {
		menuOpen = true;
	}
	function closeMenu() {
		menuOpen = false;
	}
	function onKey(e: KeyboardEvent) {
		if (e.key === 'Escape' && menuOpen) closeMenu();
	}

	// Auto-close on navigation — pathname changes via SPA back/forward
	// or any link tap. Reading `pathname` inside the effect makes it the
	// reactive trigger; we don't need to compare values.
	$effect(() => {
		void pathname;
		menuOpen = false;
	});

	function onLogout() {
		closeMenu();
		void logout();
	}
</script>

<svelte:window onkeydown={onKey} />

<header class="topbar">
	<a class="brand" href="/expenses" aria-label="Copro Manager — accueil">
		<span class="brand-mark">C/M</span>
		<span class="brand-name">Copro <em>Manager</em></span>
	</a>

	<nav class="nav-desktop" aria-label="Navigation principale">
		{#each items as item (item.href)}
			{@const active = isActive(item.href)}
			<a
				class="nav-link"
				class:active
				href={item.href}
				aria-current={active ? 'page' : undefined}
			>
				<span>{item.label}</span>
				{#if active}
					<span class="nav-underline" aria-hidden="true">⌇</span>
				{/if}
			</a>
		{/each}
	</nav>

	<div class="actions">
		{#if $authState.status === 'signed-in'}
			<AlertsBell />
		{/if}
		<div class="user-desktop">
			{#if $authState.status === 'signed-in'}
				<a class="user-email" href="/profile" title={$authState.user.email}>
					{$authState.user.email}
				</a>
				<button class="logout" type="button" onclick={() => logout()}>Déconnexion</button>
			{/if}
		</div>
		<div class="burger">
			<IconButton
				icon="menu"
				aria-label="Ouvrir le menu"
				variant="ghost"
				onclick={openMenu}
				aria-expanded={menuOpen}
				aria-controls="topbar-menu"
			/>
		</div>
	</div>
</header>

{#if menuOpen}
	<div
		class="sheet-backdrop"
		role="presentation"
		onclick={closeMenu}
		onkeydown={onKey}
	></div>
	<div
		id="topbar-menu"
		class="sheet"
		role="dialog"
		aria-modal="true"
		aria-label="Menu de navigation"
	>
		<header class="sheet-head">
			<span class="sheet-eyebrow">Menu</span>
			<IconButton
				icon="close"
				aria-label="Fermer le menu"
				variant="text"
				onclick={closeMenu}
			/>
		</header>

		<nav class="nav-sheet" aria-label="Navigation">
			{#each items as item (item.href)}
				{@const active = isActive(item.href)}
				<a
					class="sheet-link"
					class:active
					href={item.href}
					aria-current={active ? 'page' : undefined}
					onclick={closeMenu}
				>
					{#if active}<span class="sheet-mark" aria-hidden="true">⌇</span>{/if}
					<span class="sheet-label">{item.label}</span>
				</a>
			{/each}
		</nav>

		{#if $authState.status === 'signed-in'}
			<div class="sheet-foot">
				<a class="sheet-email" href="/profile" title={$authState.user.email} onclick={closeMenu}>
					{$authState.user.email}
				</a>
				<button class="sheet-logout" type="button" onclick={onLogout}>Déconnexion</button>
			</div>
		{/if}
	</div>
{/if}

<style>
	.topbar {
		max-width: 920px;
		margin: 0 auto;
		padding: 1.4rem 1.25rem 0.5rem;
		display: grid;
		grid-template-columns: auto 1fr auto;
		align-items: center;
		gap: 1.25rem;
	}

	.brand {
		display: inline-flex;
		align-items: center;
		gap: 0.6rem;
		text-decoration: none;
		color: var(--ink);
		transition: opacity var(--dur-fast) var(--ease-out);
	}
	.brand:hover {
		opacity: 0.78;
	}
	.brand-mark {
		font-family: var(--display);
		font-style: italic;
		font-size: 1rem;
		padding: 0.42rem 0.55rem;
		border: 1px solid var(--ink);
		border-radius: 999px;
		line-height: 1;
	}
	.brand-name {
		font-family: var(--display);
		font-size: 1.05rem;
	}
	.brand-name em {
		font-style: italic;
		color: var(--ink-2);
	}

	.nav-desktop {
		display: inline-flex;
		justify-content: center;
		gap: 0.4rem;
		flex-wrap: wrap;
	}
	.nav-link {
		position: relative;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		padding: 0.45rem 0.85rem;
		font-family: var(--ui);
		font-size: 0.82rem;
		font-weight: 500;
		letter-spacing: 0.01em;
		color: var(--ink-3);
		text-decoration: none;
		border-radius: 999px;
		transition:
			color var(--dur-fast) var(--ease-out),
			background-color var(--dur-fast) var(--ease-out);
	}
	.nav-link:hover {
		color: var(--ink);
		background: var(--bg-warm);
	}
	.nav-link:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
	}
	.nav-link.active {
		color: var(--ink);
		font-weight: 600;
	}
	.nav-underline {
		position: absolute;
		left: 50%;
		bottom: -0.15rem;
		transform: translateX(-50%) rotate(-10deg);
		font-family: var(--display);
		font-style: italic;
		font-size: 0.85rem;
		color: var(--accent);
		line-height: 0;
		pointer-events: none;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 0.6rem;
	}
	.user-desktop {
		display: flex;
		flex-direction: column;
		align-items: flex-end;
		gap: 0.15rem;
		min-width: 0;
	}
	.user-email {
		font-size: 0.74rem;
		color: var(--ink-3);
		max-width: 14rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-decoration: none;
		transition: color var(--dur-fast) var(--ease-out);
	}
	.user-email:hover {
		color: var(--ink);
	}
	.logout {
		background: transparent;
		border: 0;
		color: var(--accent);
		font-family: var(--ui);
		font-size: 0.76rem;
		font-weight: 500;
		cursor: pointer;
		padding: 0;
		text-decoration: underline;
		text-underline-offset: 3px;
		text-decoration-color: rgba(194, 78, 42, 0.35);
		transition:
			color var(--dur-fast) var(--ease-out),
			text-decoration-color var(--dur-fast) var(--ease-out);
	}
	.logout:hover {
		color: var(--accent-deep);
		text-decoration-color: currentColor;
	}
	.logout:focus-visible {
		outline: 2px solid var(--accent);
		outline-offset: 2px;
		border-radius: 4px;
	}

	.burger {
		display: none;
	}

	/* ─── Mobile breakpoint ─────────────────────────────────────────── */
	@media (max-width: 720px) {
		.topbar {
			grid-template-columns: 1fr auto;
			padding: 0.85rem 1rem 0.5rem;
		}
		.brand-name {
			display: none;
		}
		.nav-desktop,
		.user-desktop {
			display: none;
		}
		.burger {
			display: block;
		}
		.actions {
			gap: 0.4rem;
		}
	}

	/* ─── Mobile sheet ──────────────────────────────────────────────── */
	.sheet-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.32);
		backdrop-filter: blur(4px);
		-webkit-backdrop-filter: blur(4px);
		z-index: 90;
		animation: fade-in var(--dur-base) var(--ease-out);
	}
	.sheet {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		z-index: 100;
		background: var(--surface);
		border-bottom: 1px solid var(--hairline);
		box-shadow: 0 24px 48px rgba(20, 16, 12, 0.18);
		padding: 1rem 1.1rem 1.4rem;
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		animation: slide-down var(--dur-base) var(--ease-out);
	}
	.sheet-head {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.2rem 0 0.6rem;
		border-bottom: 1px solid var(--hairline);
		margin-bottom: 0.4rem;
	}
	.sheet-eyebrow {
		font-size: 0.7rem;
		text-transform: uppercase;
		letter-spacing: 0.18em;
		color: var(--ink-4);
		font-weight: 600;
	}

	.nav-sheet {
		display: flex;
		flex-direction: column;
	}
	.sheet-link {
		position: relative;
		display: flex;
		align-items: center;
		gap: 0.6rem;
		padding: 0.95rem 0.6rem;
		text-decoration: none;
		color: var(--ink-2);
		font-family: var(--ui);
		font-size: 1rem;
		font-weight: 500;
		border-radius: 0.6rem;
		transition: background-color var(--dur-fast) var(--ease-out);
	}
	.sheet-link:hover,
	.sheet-link:focus-visible {
		background: var(--bg-warm);
		color: var(--ink);
		outline: none;
	}
	.sheet-link.active {
		color: var(--ink);
		font-weight: 600;
	}
	.sheet-mark {
		font-family: var(--display);
		font-style: italic;
		font-size: 1.15rem;
		color: var(--accent);
		transform: rotate(-15deg);
		line-height: 0;
	}
	.sheet-link:not(.active) .sheet-label {
		padding-left: 1.5rem;
	}

	.sheet-foot {
		margin-top: 0.5rem;
		padding-top: 0.85rem;
		border-top: 1px solid var(--hairline);
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}
	.sheet-email {
		font-size: 0.78rem;
		color: var(--ink-3);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		text-decoration: none;
	}
	.sheet-email:hover {
		color: var(--ink);
	}
	.sheet-logout {
		align-self: flex-start;
		background: transparent;
		border: 1px solid rgba(194, 78, 42, 0.3);
		color: var(--accent);
		font-family: var(--ui);
		font-size: 0.85rem;
		font-weight: 600;
		padding: 0.55rem 1rem;
		border-radius: 999px;
		cursor: pointer;
		transition:
			background-color var(--dur-fast) var(--ease-out),
			border-color var(--dur-fast) var(--ease-out),
			color var(--dur-fast) var(--ease-out);
	}
	.sheet-logout:hover {
		background: var(--accent-soft);
		border-color: var(--accent);
		color: var(--accent-deep);
	}

	@keyframes slide-down {
		from {
			transform: translateY(-100%);
			opacity: 0;
		}
		to {
			transform: translateY(0);
			opacity: 1;
		}
	}
	@keyframes fade-in {
		from {
			opacity: 0;
		}
		to {
			opacity: 1;
		}
	}
</style>
