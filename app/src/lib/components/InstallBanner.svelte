<script lang="ts">
	import {
		detectPlatform,
		dismissBanner,
		isBannerDismissed,
		isStandalone,
		onInstallAvailability,
		promptInstall,
		type PwaPlatform
	} from '$lib/pwa';

	let show = $state(false);
	let platform = $state<PwaPlatform>('unknown');
	let nativePromptAvailable = $state(false);
	let busy = $state(false);

	$effect(() => {
		if (typeof window === 'undefined') return;
		platform = detectPlatform();
		const standalone = isStandalone();
		const dismissed = isBannerDismissed();
		// Banner only shows when the app is running in browser mode AND
		// the user hasn't dismissed it for this session.
		show = !standalone && !dismissed;

		const unsub = onInstallAvailability((avail) => (nativePromptAvailable = avail));
		return () => unsub();
	});

	async function onInstall() {
		if (busy) return;
		busy = true;
		try {
			const outcome = await promptInstall();
			if (outcome === 'accepted') {
				show = false;
			}
		} catch {
			// Native prompt unavailable — leave the banner up; instructions
			// stay visible for the user to install manually.
		} finally {
			busy = false;
		}
	}

	function onDismiss() {
		dismissBanner();
		show = false;
	}
</script>

{#if show}
	<aside class="install-banner" role="region" aria-label="Installer l'application">
		<div class="install-banner-body">
			<p class="install-banner-title">Installer Copro Manager</p>
			{#if platform === 'ios'}
				<p class="install-banner-text">
					Touchez <span class="ib-glyph">⎙</span> dans la barre d'outils Safari, puis
					<strong>« Sur l'écran d'accueil »</strong> pour ajouter l'app.
				</p>
			{:else if platform === 'android'}
				<p class="install-banner-text">
					{nativePromptAvailable
						? 'Tapez « Installer » pour ajouter l\'app à votre écran d\'accueil.'
						: 'Ouvrez le menu de Chrome (⋮) puis « Installer l\'application ».'}
				</p>
			{:else}
				<p class="install-banner-text">
					{nativePromptAvailable
						? 'Tapez « Installer » pour ajouter l\'app à votre navigateur.'
						: 'Cliquez sur l\'icône d\'installation dans la barre d\'URL ou ouvrez le menu navigateur.'}
				</p>
			{/if}
		</div>
		<div class="install-banner-actions">
			{#if nativePromptAvailable}
				<button
					type="button"
					class="ib-btn ib-btn-primary"
					onclick={onInstall}
					disabled={busy}
				>
					{busy ? '…' : 'Installer'}
				</button>
			{/if}
			<button type="button" class="ib-btn ib-btn-ghost" onclick={onDismiss} aria-label="Ignorer">
				✕
			</button>
		</div>
	</aside>
{/if}

<style>
	.install-banner {
		position: fixed;
		left: 50%;
		bottom: 1rem;
		transform: translateX(-50%);
		width: min(440px, calc(100vw - 1.5rem));
		display: flex;
		align-items: center;
		gap: 0.85rem;
		padding: 0.75rem 0.85rem 0.75rem 1rem;
		background: #ffffff;
		border: 1px solid #d8d0c1;
		border-radius: 0.85rem;
		box-shadow:
			0 18px 40px rgba(20, 16, 12, 0.18),
			0 4px 12px rgba(20, 16, 12, 0.08);
		z-index: 60;
		font-family:
			'Hanken Grotesk',
			-apple-system,
			BlinkMacSystemFont,
			'Segoe UI',
			system-ui,
			sans-serif;
		color: #161310;
		animation: ib-slide-in 240ms cubic-bezier(0.2, 0.8, 0.2, 1);
	}
	@keyframes ib-slide-in {
		from {
			transform: translate(-50%, 12px);
			opacity: 0;
		}
		to {
			transform: translate(-50%, 0);
			opacity: 1;
		}
	}
	.install-banner-body {
		flex: 1;
		min-width: 0;
	}
	.install-banner-title {
		font-family: 'Fraunces', 'Hoefler Text', Georgia, serif;
		font-weight: 500;
		font-size: 0.95rem;
		margin: 0 0 0.15rem;
	}
	.install-banner-text {
		font-size: 0.78rem;
		color: #44403a;
		margin: 0;
		line-height: 1.35;
	}
	.ib-glyph {
		display: inline-block;
		font-size: 0.95rem;
		color: #c24e2a;
		transform: translateY(1px);
	}
	.install-banner-actions {
		display: flex;
		align-items: center;
		gap: 0.4rem;
		flex-shrink: 0;
	}
	.ib-btn {
		font-family: inherit;
		font-size: 0.78rem;
		font-weight: 600;
		border-radius: 999px;
		cursor: pointer;
		padding: 0.42rem 0.85rem;
		border: 1px solid transparent;
	}
	.ib-btn-primary {
		background: #161310;
		color: #faf8f4;
		border-color: #161310;
	}
	.ib-btn-primary:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
	.ib-btn-ghost {
		background: transparent;
		color: #7a7268;
		font-size: 1rem;
		padding: 0.4rem 0.5rem;
	}
	.ib-btn-ghost:hover {
		color: #161310;
	}
	@media (max-width: 480px) {
		.install-banner {
			bottom: 0.55rem;
			padding: 0.6rem 0.7rem 0.6rem 0.85rem;
		}
	}
</style>
