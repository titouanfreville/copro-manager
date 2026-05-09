import { sveltekit } from "@sveltejs/kit/vite";
import { SvelteKitPWA } from "@vite-pwa/sveltekit";
import { defineConfig } from "vite";

export default defineConfig(({ command }) => ({
  plugins: [
    sveltekit(),
    SvelteKitPWA({
      // `autoUpdate` posts a new SW on every deploy; `immediate: false`
      // means the new SW takes over on the next navigation rather than
      // hard-reloading the page mid-form. We don't wire `onNeedRefresh`
      // because the 2-foyer scope tolerates a one-navigation lag — a
      // full app reload mid-create would nuke the user's typed form.
      registerType: "autoUpdate",
      // `injectManifest` instead of `generateSW` so we can ship our
      // own SW source (push + notificationclick handlers). SvelteKit
      // compiles `src/service-worker.ts` to
      // `.svelte-kit/output/client/service-worker.js` automatically;
      // we just need to point the plugin at the compiled output's
      // default name. Workbox replaces `self.__WB_MANIFEST` with the
      // precache list at build time.
      strategies: "injectManifest",
      injectManifest: {
        globPatterns: ["**/*.{js,css,html,svg,png,ico,webp}"],
      },
      manifest: {
        name: "Copro Manager",
        short_name: "Copro",
        description: "Gestion de copropriété pour 2 foyers",
        // Match `<meta name="theme-color">` in app.html so Android's
        // splash doesn't flash a different background on cold start.
        theme_color: "#faf8f4",
        background_color: "#faf8f4",
        display: "standalone",
        start_url: "/",
        scope: "/",
        icons: [
          { src: "/icon-192.png", sizes: "192x192", type: "image/png" },
          { src: "/icon-512.png", sizes: "512x512", type: "image/png" },
          {
            src: "/icon-maskable.png",
            sizes: "512x512",
            type: "image/png",
            purpose: "maskable",
          },
        ],
      },
      // Disable the dev-mode service worker. Active SW + Vite HMR is
      // a known footgun (stale modules, infinite reload loops). The
      // production build still ships the SW.
      devOptions: {
        enabled: command === "build",
        type: "module",
      },
    }),
  ],
  server: {
    port: 5173,
  },
}));
