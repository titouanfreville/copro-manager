import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";

// Derive the API origin from the build-time public env var so the CSP
// connect-src directive lets the SvelteKit bundle reach it. Falls back
// to the local dev API when the var isn't set (e.g. PR builds).
const apiOrigin = (() => {
  const raw = process.env.PUBLIC_API_BASE_URL || "http://localhost:8080";
  try {
    return new URL(raw).origin;
  } catch {
    return "http://localhost:8080";
  }
})();

/** @type {import('@sveltejs/kit').Config} */
const config = {
  preprocess: vitePreprocess(),
  kit: {
    adapter: adapter({
      pages: "build",
      assets: "build",
      fallback: "index.html",
      precompress: false,
      strict: true,
    }),
    alias: {
      $lib: "src/lib",
    },
    // Inline scripts SvelteKit emits for hydration change every build,
    // so we let it compute their hashes and inject a <meta> CSP. The
    // header-level CSP in firebase.json keeps only directives that
    // can't live in meta (frame-ancestors).
    csp: {
      mode: "hash",
      directives: {
        "default-src": ["self"],
        "script-src": [
          "self",
          "wasm-unsafe-eval",
          "https://www.googletagmanager.com",
          "https://apis.google.com",
          "https://www.gstatic.com",
        ],
        "style-src": ["self", "unsafe-inline", "https://fonts.googleapis.com"],
        "img-src": [
          "self",
          "data:",
          "blob:",
          "https://storage.googleapis.com",
          "https://*.googleusercontent.com",
        ],
        "font-src": ["self", "data:", "https://fonts.gstatic.com"],
        "connect-src": [
          "self",
          apiOrigin,
          "https://*.googleapis.com",
          "https://*.firebaseio.com",
          "https://*.firebaseapp.com",
          "https://identitytoolkit.googleapis.com",
          "https://securetoken.googleapis.com",
          "https://firestore.googleapis.com",
          "https://storage.googleapis.com",
          "wss://*.firebaseio.com",
        ],
        "frame-src": ["self", "https://*.firebaseapp.com"],
        "worker-src": ["self", "blob:"],
        "manifest-src": ["self"],
        "base-uri": ["self"],
        "form-action": ["self"],
        "object-src": ["none"],
      },
    },
  },
};

export default config;
