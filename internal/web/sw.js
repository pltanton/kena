const BUILD = "__BUILD__";
const ASSETS = __ASSETS__;
const STATIC = "kena-static-" + BUILD;
const PAGES = "kena-pages";
const TIMEOUT = 3000;

function pageKey(url) {
  if (url.pathname === "/") return "/";
  var m = url.pathname.match(/^\/s\/([^/]+)(\/play)?$/);
  return m ? "/s/" + m[1] + (m[2] || "") : null;
}

function network(request) {
  return new Promise(function (resolve, reject) {
    var timer = setTimeout(function () { reject(new Error("timeout")); }, TIMEOUT);
    fetch(request).then(function (r) {
      clearTimeout(timer);
      resolve(r);
    }, function (err) {
      clearTimeout(timer);
      reject(err);
    });
  });
}

function marked(response) {
  return response.text().then(function (html) {
    return new Response(html.replace("<html ", '<html data-offline="1" '), {status: 200, headers: response.headers});
  });
}

self.addEventListener("install", function (e) {
  e.waitUntil(caches.open(STATIC).then(function (c) {
    return c.addAll(ASSETS.map(function (u) { return new Request(u, {cache: "reload"}); }));
  }).then(function () {
    return caches.open(PAGES).then(function (c) { return c.add("/"); }).catch(function () {});
  }).then(function () { return self.skipWaiting(); }));
});

self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (keys) {
    return Promise.all(keys.filter(function (k) { return k.indexOf("kena-static-") === 0 && k !== STATIC; }).map(function (k) { return caches.delete(k); }));
  }).then(function () {
    return self.clients.claim();
  }).then(function () {
    return caches.open(PAGES).then(function (c) {
      return c.keys().then(function (reqs) {
        return Promise.all(reqs.map(function (req) {
          return fetch(req.url, {credentials: "same-origin"}).then(function (r) {
            if (r.ok) return c.put(req, r);
          }).catch(function () {});
        }));
      });
    });
  }));
});

function navigate(request, url) {
  var key = pageKey(url);
  return network(request).then(function (response) {
    if (key && response.ok && response.status === 200 && !response.redirected) {
      var copy = response.clone();
      caches.open(PAGES).then(function (c) {
        return (key === "/" ? Promise.resolve(true) : c.match(key)).then(function (have) {
          if (have) return c.put(key, copy);
        });
      });
    }
    return response;
  }, function () {
    return caches.open(PAGES).then(function (c) {
      return key ? c.match(key) : undefined;
    }).then(function (hit) {
      if (hit) return marked(hit);
      return caches.match("/offline").then(function (page) {
        return page ? marked(page) : Response.error();
      });
    });
  });
}

function asset(request) {
  return network(request).catch(function (err) {
    return caches.match(request, {ignoreSearch: true}).then(function (hit) {
      if (hit) return hit;
      throw err;
    });
  });
}

self.addEventListener("fetch", function (e) {
  var request = e.request;
  if (request.method !== "GET") return;
  var url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (request.mode === "navigate") {
    e.respondWith(navigate(request, url));
  } else if (url.pathname.indexOf("/static/") === 0) {
    e.respondWith(asset(request));
  }
});

