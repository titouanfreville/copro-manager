import { sveltekit } from '@sveltejs/kit/vite';
import { SvelteKitPWA } from '@vite-pwa/sveltekit';
import { defineConfig } from 'vite';

export default defineConfig(({ command }) => ({
	plugins: [
		sveltekit(),
		SvelteKitPWA({
			// `autoUpdate` posts a new SW on every deploy; `immediate: false`
			// means the new SW takes over on the next navigation rather than
			// hard-reloading the page mid-form. We don't wire `onNeedRefresh`
			// because the 2-foyer scope tolerates a one-navigation lag — a
			// full app reload mid-create would nuke the user's typed form.
			registerType: 'autoUpdate',
			strategies: 'generateSW',
			manifest: {
				name: 'Copro Manager',
				short_name: 'Copro',
				description: 'Gestion de copropriété pour 2 foyers',
				theme_color: '#0f172a',
				background_color: '#0f172a',
				display: 'standalone',
				start_url: '/',
				scope: '/',
				icons: [
					{ src: '/icon-192.png', sizes: '192x192', type: 'image/png' },
					{ src: '/icon-512.png', sizes: '512x512', type: 'image/png' },
					{
						src: '/icon-maskable.png',
						sizes: '512x512',
						type: 'image/png',
						purpose: 'maskable'
					}
				]
			},
			workbox: {
				globPatterns: ['**/*.{js,css,html,svg,png,ico,webp}'],
				runtimeCaching: [
					{
						// GCS document URLs are signed with a short TTL (≤1h per
						// NFR12). `CacheFirst` would happily serve a stale 403
						// long after the signature expired; `NetworkFirst` falls
						// back to cache only when offline.
						urlPattern: /^https:\/\/storage\.googleapis\.com\/.*/i,
						handler: 'NetworkFirst',
						options: {
							cacheName: 'gcs-documents',
							networkTimeoutSeconds: 5,
							expiration: { maxEntries: 64, maxAgeSeconds: 60 * 60 }
						}
					}
				]
			},
			// Disable the dev-mode service worker. Active SW + Vite HMR is
			// a known footgun (stale modules, infinite reload loops). The
			// production build still ships the SW.
			devOptions: {
				enabled: command === 'build',
				type: 'module'
			}
		})
	],
	server: {
		port: 5173
	}
}));
