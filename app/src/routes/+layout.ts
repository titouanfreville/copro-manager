// Static SPA — no SSR.
export const ssr = false;
export const prerender = false;
export const trailingSlash = "never";

// Dev-mode: nuke any service worker that a prior `npm run build` /
// `npm run preview` registered against localhost. The vite-pwa plugin
// keeps dev-mode SW disabled (HMR + SW = stale-module nightmare), but
// the browser will keep re-evaluating the cached prod SW on every dev
// reload until it's explicitly unregistered. Running this at module
// eval (before any component mounts) catches it as early as we can
// from inside SvelteKit.
//
// Note: the FIRST reload after this lands still logs one round of
// 404s on /service-worker.js + /manifest.webmanifest because the
// browser's SW machinery wakes up before any of our JS runs. The
// SECOND reload is clean. To skip the wait, unregister manually via
// DevTools → Application → Service Workers → Unregister.
if (
  typeof window !== "undefined" &&
  import.meta.env.DEV &&
  "serviceWorker" in navigator
) {
  void navigator.serviceWorker.getRegistrations().then((regs) => {
    for (const reg of regs) {
      void reg.unregister();
    }
  });
}
