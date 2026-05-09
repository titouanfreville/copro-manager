<!--
	Webcam capture button + modal. Complements <input type="file" capture>
	which already triggers the system camera on mobile but is ignored on
	desktop. This component fills the desktop gap via getUserMedia.

	Hidden when getUserMedia is unavailable (older browsers, insecure
	contexts). The file input remains the fallback path.
-->
<script lang="ts">
	import Button from './Button.svelte';

	type Props = {
		onCapture: (file: File) => void;
		filename?: string;
		facingMode?: 'environment' | 'user';
		disabled?: boolean;
		label?: string;
	};

	let {
		onCapture,
		filename = 'photo.jpg',
		facingMode = 'environment',
		disabled = false,
		label = 'Photo'
	}: Props = $props();

	let supported = $state(false);
	let open = $state(false);
	let stream = $state<MediaStream | null>(null);
	let videoEl = $state<HTMLVideoElement | null>(null);
	let error = $state('');
	let busy = $state(false);
	// svelte-ignore state_referenced_locally
	let currentFacing = $state<'environment' | 'user'>(facingMode);

	$effect(() => {
		supported =
			typeof navigator !== 'undefined' &&
			!!navigator.mediaDevices &&
			typeof navigator.mediaDevices.getUserMedia === 'function';
	});

	async function start(face: 'environment' | 'user') {
		error = '';
		stop();
		try {
			stream = await navigator.mediaDevices.getUserMedia({
				video: { facingMode: { ideal: face } },
				audio: false
			});
			currentFacing = face;
			// Attach the stream once the <video> element is in the DOM.
			queueMicrotask(() => {
				if (videoEl && stream) {
					videoEl.srcObject = stream;
				}
			});
		} catch (err) {
			const e = err as DOMException;
			if (e?.name === 'NotAllowedError') {
				error = "Accès caméra refusé. Autorise l'accès dans le navigateur.";
			} else if (e?.name === 'NotFoundError') {
				error = 'Aucune caméra détectée sur cet appareil.';
			} else {
				error = e?.message || 'Impossible de démarrer la caméra.';
			}
		}
	}

	function stop() {
		if (stream) {
			for (const t of stream.getTracks()) t.stop();
			stream = null;
		}
		if (videoEl) videoEl.srcObject = null;
	}

	async function openModal() {
		if (disabled || busy) return;
		open = true;
		await start(currentFacing);
	}

	function closeModal() {
		open = false;
		stop();
		error = '';
	}

	async function capture() {
		if (!videoEl || !stream || busy) return;
		busy = true;
		try {
			const w = videoEl.videoWidth;
			const h = videoEl.videoHeight;
			if (!w || !h) {
				error = 'Image non prête, réessaie.';
				return;
			}
			const canvas = document.createElement('canvas');
			canvas.width = w;
			canvas.height = h;
			const ctx = canvas.getContext('2d');
			if (!ctx) {
				error = 'Canvas indisponible.';
				return;
			}
			ctx.drawImage(videoEl, 0, 0, w, h);
			const blob: Blob | null = await new Promise((resolve) =>
				canvas.toBlob((b) => resolve(b), 'image/jpeg', 0.9)
			);
			if (!blob) {
				error = 'Capture impossible.';
				return;
			}
			const stamp = new Date().toISOString().replace(/[:.]/g, '-');
			const name = filename.replace(/\.jpe?g$/i, '') + `-${stamp}.jpg`;
			const file = new File([blob], name, { type: 'image/jpeg' });
			onCapture(file);
			closeModal();
		} finally {
			busy = false;
		}
	}

	function flip() {
		const next = currentFacing === 'environment' ? 'user' : 'environment';
		start(next);
	}

	function onKeydown(e: KeyboardEvent) {
		if (e.key === 'Escape' && open) closeModal();
	}
</script>

<svelte:window onkeydown={onKeydown} />

{#if supported}
	<Button
		variant="ghost"
		size="sm"
		type="button"
		{disabled}
		onclick={openModal}
		aria-label="Prendre une photo"
	>
		<span class="cam" aria-hidden="true">📷</span>
		{label}
	</Button>
{/if}

{#if open}
	<div
		class="pc-backdrop"
		role="button"
		tabindex="-1"
		aria-label="Fermer"
		onclick={closeModal}
		onkeydown={(e) => {
			if (e.key === 'Enter' || e.key === ' ') closeModal();
		}}
	></div>
	<div class="pc-modal" role="dialog" aria-modal="true" aria-label="Prendre une photo">
		<div class="pc-head">
			<h2>Prendre une photo</h2>
			<button type="button" class="pc-close" onclick={closeModal} aria-label="Fermer">×</button>
		</div>
		<div class="pc-body">
			{#if error}
				<p class="pc-error" role="alert">{error}</p>
			{/if}
			<div class="pc-stage">
				<!-- svelte-ignore a11y_media_has_caption -->
				<video
					bind:this={videoEl}
					autoplay
					playsinline
					muted
					class:mirrored={currentFacing === 'user'}
				></video>
			</div>
			<div class="pc-actions">
				<Button variant="ghost" type="button" onclick={closeModal} disabled={busy}>
					Annuler
				</Button>
				<Button variant="ghost" type="button" onclick={flip} disabled={busy || !stream}>
					Inverser
				</Button>
				<Button
					variant="primary"
					type="button"
					mark
					onclick={capture}
					disabled={busy || !stream}
				>
					{busy ? 'Capture…' : 'Capturer'}
				</Button>
			</div>
		</div>
	</div>
{/if}

<style>
	.cam {
		font-size: 1rem;
		line-height: 1;
	}

	.pc-backdrop {
		position: fixed;
		inset: 0;
		background: rgba(20, 16, 12, 0.55);
		backdrop-filter: blur(4px);
		z-index: 90;
		border: 0;
		padding: 0;
		cursor: pointer;
	}
	.pc-modal {
		position: fixed;
		left: 50%;
		top: 50%;
		transform: translate(-50%, -50%);
		width: min(640px, calc(100vw - 1.5rem));
		max-height: calc(100vh - 2rem);
		overflow: auto;
		background: var(--surface);
		border: 1px solid var(--hairline);
		border-radius: 1.1rem;
		box-shadow: 0 24px 60px rgba(20, 16, 12, 0.25);
		z-index: 100;
		display: flex;
		flex-direction: column;
	}
	.pc-head {
		padding: 1rem 1.2rem 0.4rem;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
	}
	.pc-head h2 {
		font-family: var(--display);
		font-weight: 400;
		font-size: 1.2rem;
		margin: 0;
	}
	.pc-close {
		background: transparent;
		border: 0;
		font-size: 1.4rem;
		line-height: 1;
		color: var(--ink-3);
		cursor: pointer;
		padding: 0.25rem 0.45rem;
		border-radius: 999px;
	}
	.pc-close:hover {
		color: var(--ink);
		background: var(--bg-warm);
	}
	.pc-body {
		padding: 0.4rem 1.2rem 1.1rem;
		display: flex;
		flex-direction: column;
		gap: 0.8rem;
	}
	.pc-stage {
		background: #000;
		border-radius: 0.8rem;
		overflow: hidden;
		aspect-ratio: 4 / 3;
		display: flex;
		align-items: center;
		justify-content: center;
	}
	.pc-stage video {
		width: 100%;
		height: 100%;
		object-fit: cover;
		display: block;
	}
	.pc-stage video.mirrored {
		transform: scaleX(-1);
	}
	.pc-actions {
		display: flex;
		gap: 0.5rem;
		justify-content: flex-end;
		flex-wrap: wrap;
	}
	.pc-error {
		margin: 0;
		padding: 0.55rem 0.75rem;
		background: rgba(183, 50, 35, 0.08);
		border: 1px solid rgba(183, 50, 35, 0.25);
		border-radius: 0.5rem;
		color: var(--danger);
		font-size: 0.85rem;
	}
</style>
