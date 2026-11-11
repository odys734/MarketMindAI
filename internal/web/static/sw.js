const CACHE="marketmind-shell-v1";
const SHELL=["/app/","/app/index.html","/app/styles.css","/app/app.js","/app/manifest.webmanifest"];
self.addEventListener("install",event=>event.waitUntil(caches.open(CACHE).then(cache=>cache.addAll(SHELL)).then(()=>self.skipWaiting())));
self.addEventListener("activate",event=>event.waitUntil(caches.keys().then(keys=>Promise.all(keys.filter(key=>key!==CACHE).map(key=>caches.delete(key)))).then(()=>self.clients.claim())));
self.addEventListener("fetch",event=>{const url=new URL(event.request.url);if(event.request.method!=="GET"||url.pathname.startsWith("/api/"))return;if(url.origin===self.location.origin&&url.pathname.startsWith("/app/"))event.respondWith(caches.match(event.request).then(cached=>cached||fetch(event.request).then(response=>{const copy=response.clone();caches.open(CACHE).then(cache=>cache.put(event.request,copy));return response;})));});
