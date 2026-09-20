/* Light installable shell: cache the app chrome so a revisit still paints. */
const CACHE = "school-nanny-shell-v44";
const SHELL = [
  "/static/app.css",
  "/static/pico.min.css",
  "/static/htmx.min.js",
  "/static/theme.js",
  "/static/ui.js",
  "/static/planner.js",
  "/static/calendar.js",
  "/static/history.js",
  "/static/pdf-viewer.js",
  "/static/pdfjs/pdf.min.mjs",
  "/static/pdfjs/pdf.worker.min.mjs",
  "/static/icon-192.png",
  "/static/manifest.webmanifest"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") {
    return;
  }
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) {
    return;
  }
  if (!url.pathname.startsWith("/static/")) {
    return;
  }
  event.respondWith(
    caches.match(req).then((cached) => {
      const networked = fetch(req).then((res) => {
        if (res && res.ok) {
          const copy = res.clone();
          caches.open(CACHE).then((cache) => cache.put(req, copy));
        }
        return res;
      }).catch(() => cached);
      return cached || networked;
    })
  );
});
