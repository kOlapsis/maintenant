# Auth-proxy lab

Maintenant behind an OIDC login, to exercise the session-expiry handling in
`frontend/src/services/authGuard.ts` without a VM and without touching DNS.

    docker compose -f compose.authproxy.yml up -d
    # http://localhost:8088 → admin@lab.test / password

- `oauth2-proxy` fronts the app; nothing else is published.
- `dex` is the OIDC provider: in-memory, one static user, restart wipes it.
- Add `--build` to rebuild the app image after a frontend change.

Everything is on `localhost` on purpose. A service worker only registers on a
secure origin, and `http://anything.local` is not one — on a `.local` name the
app shell is never cached, and the bug this lab reproduces (cached shell booting
on a dead session) cannot happen at all. A `.local` variant would need HTTPS and
a CA trusted by the browser.

## Killing the session

The session lives in the `_oauth2_proxy` cookie. From the browser console:

```js
document.cookie.split(';').map(c => c.trim().split('=')[0])
  .filter(n => n.startsWith('_oauth2_proxy'))
  .forEach(n => { document.cookie = `${n}=; Max-Age=0; path=/` })
```

The cookie is deliberately not `HttpOnly` here so that one-liner works. To watch
a session die on its own instead:

    AUTHLAB_COOKIE_EXPIRE=1m docker compose -f compose.authproxy.yml up -d

## Scenarios

1. **Expiry while the tab is open** — kill the cookie, then let the app poll or
   click through a page. Expected: the overlay, then a round-trip to dex.
2. **Boot on a dead session** — kill the cookie, then reload. This is the
   original bug: the service worker replays the cached shell, so the SPA boots
   with no session at all.
3. **Re-auth that does not stick** — kill the cookie again within 15s of the
   previous round-trip. The loop guard fires: the overlay stays put in its
   stalled state, with a "Sign in again" button, instead of bouncing forever.
4. **A JSON 403 is not an expired session** — the lab runs Community edition, so
   Pro endpoints answer `403 PRO_REQUIRED`. Nothing should ever appear.

## Two flavours of challenge

By default the proxy answers unauthenticated API calls with a `302` to dex.
Because dex is another origin, the browser blocks the redirected response on
CORS and `fetch` rejects — which is what sends `authGuard` to `probeAuth()`.

Uncomment `OAUTH2_PROXY_API_ROUTES` in the compose file to get a bare `401` on
`/api/` instead, the other branch of `isAuthChallenge()`.
