/*
 * Service worker — caches the shell only (HTML/CSS/JS/icons).
 *
 * API responses are never cached: every byte of this app's data is read live
 * from the local disk, and showing a stale answer would only mislead. The cache
 * exists so an installed PWA opens instantly.
 */

// The version is baked into the cache name at startup by the server. A new
// binary means a new name, and activate deletes the old one — otherwise the
// previous UI would live on inside an installed PWA.
const CACHE = 'kisaf-shell-__KISAF_VERSION__';
const SHELL = [
  '/',
  '/style.css',
  '/app.js',
  '/i18n.js',
  '/manifest.webmanifest',
  '/icons/favicon-32.png',
  '/icons/icon-192.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE).then((cache) => cache.addAll(SHELL)).then(() => self.skipWaiting()),
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener('fetch', (event) => {
  const { request } = event;
  const url = new URL(request.url);

  if (request.method !== 'GET' || url.origin !== self.location.origin) return;
  if (url.pathname.startsWith('/api/') || url.pathname === '/login') return;

  // Network first: while the server is up the shell is always current; when it
  // is down the page still opens and reports that it cannot reach the server.
  //
  // The fallback looks ONLY in this version's cache. A bare caches.match()
  // searches every cache, so if an older version's cache had not been deleted
  // yet, a brief outage could bring the old UI back.
  event.respondWith(
    fetch(request)
      .then((response) => {
        if (response.ok) {
          const copy = response.clone();
          caches.open(CACHE).then((cache) => cache.put(request, copy));
        }
        return response;
      })
      .catch(async () => {
        const cache = await caches.open(CACHE);
        return (await cache.match(request)) || (await cache.match('/')) || Response.error();
      }),
  );
});
