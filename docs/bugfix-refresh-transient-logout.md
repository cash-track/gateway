# Bug-fix plan: don't log users out on a transient refresh failure

## Background

The gateway auto-refreshes tokens: on a `401` from the API it calls `POST /auth/refresh`, and on success retries the original request (`service/api/forward.go`).

When the refresh does **not** succeed, the gateway currently logs the user out (deletes both auth cookies) — **regardless of why** it failed. That is correct for a genuinely expired refresh token, but wrong for a transient failure (API briefly unreachable, network blip, API `5xx`): the user's refresh token is still valid, yet a momentary hiccup discards their session.

The bug is a **conflation of two distinct outcomes** into one "empty result → delete cookies" path.

## Root cause

`refreshToken` (`service/api/refresh_token.go`) already distinguishes three outcomes, but the caller ignores the distinction:

| Outcome | Return value | Meaning |
|---|---|---|
| Success | populated `Auth`, `err == nil` | new tokens issued |
| **Clean 401** | empty `Auth`, **`err == nil`** (lines 71–76) | refresh token genuinely expired/invalid → **log out is correct** |
| **Transient/unexpected** | empty `Auth`, **`err != nil`** | `http.Do` failed (lines 61–66) or API returned non-401 e.g. `5xx` (lines 78–85) → **log out is wrong** |

`ForwardRequest` (`forward.go`) then does:

```go
newAuth, err := s.refreshToken(auth, spanCtx, ctx)
if err != nil {
    span.RecordError(err)
    log.Printf("[%s] refresh token attempt: %s", remoteIp, err.Error())   // err logged, then dropped
}

if newAuth.IsLogged() {
    // retry with new token, seed csrf
}

newAuth.WriteCookie(ctx)   // ← empty Auth ⇒ deletes both cookies, in BOTH empty cases
return forwardResponse(ctx, resp)
```

`WriteCookie` (`headers/cookie/auth.go:38-44`) deletes both cookies whenever `Auth` is not logged. Because line 145 is unconditional, the transient case (`err != nil`, empty `Auth`) lands on the exact same cookie-deletion as a real expiry.

### The bug is enshrined in a test

`service/api/forward_test.go:137` `TestForwardRequestWithAuthRefreshFailLogout` makes the refresh `Do` return `fmt.Errorf("broken pipe")` (a **transport error**, not a 401) and asserts both cookies are deleted (lines 168–169). So the current behaviour intentionally logs out on any refresh failure, including network blips. This test must be rewritten.

## Fix

Branch on `err` before touching cookies. Only delete cookies (log out) when refresh returned a **definitive** answer that the session is gone (`err == nil && !newAuth.IsLogged()`). On a transient failure (`err != nil`), preserve the cookies and return a retryable status.

### Change (`service/api/forward.go`, the block at lines ~99–147)

```go
// perform refresh token
newAuth, err := s.refreshToken(auth, spanCtx, ctx)
if err != nil {
    // Transient failure: could not reach the API or it returned a non-401
    // (e.g. 5xx). The refresh token may still be valid, so DO NOT delete
    // cookies / log the user out. Preserve the session and return a
    // retryable status; the client can retry once the API recovers.
    span.RecordError(err)
    log.Printf("[%s] refresh token attempt (transient, keeping session): %s", remoteIp, err.Error())

    resp.Reset()
    resp.SetStatusCode(fasthttp.StatusServiceUnavailable)
    return forwardResponse(ctx, resp) // WriteCookie deliberately NOT called → cookies untouched
}

if newAuth.IsLogged() {
    // ... unchanged: write bearer, retry request, seed csrf ...
}

// err == nil and not logged ⇒ refresh token genuinely expired/invalid.
// Logging out (cookie deletion) is the correct behaviour here.
newAuth.WriteCookie(ctx)
return forwardResponse(ctx, resp)
```

### Resulting behaviour matrix

| Refresh outcome | Retry? | Cookies | Client status |
|---|---|---|---|
| Success | yes | new tokens set | retried response (e.g. 200) |
| Clean 401 (expired) | no | **deleted (log out)** | original 401 |
| Transient (`err != nil`) | no | **preserved** | `503` (was: 401 + cookies deleted) |

### Decision: return `503` on transient failure (confirmed)

- Returning the **original 401** would make the SPA interceptor (`frontend/src/api/client.ts:23-25`) redirect to `/login`, even though the session is still valid — the wrong UX for a blip.
- `503` is not treated as auth failure by the client: no redirect, no logout. It surfaces as a normal "couldn't load, try again" error (and pairs with the frontend resilience work in `frontend/docs/bugfix-wallets-load-resilience.md`).
- Consider adding a short `Retry-After` header. Optional.

> Rejected alternative (kept for the record): keep returning the original 401 but remove the cookie deletion. This still fixes the wrongful logout, but bounces the user to login on a blip instead of letting them retry in place.

## Tests (`service/api/forward_test.go`)

1. **Rewrite** `TestForwardRequestWithAuthRefreshFailLogout`:
   - Rename to `TestForwardRequestWithAuthRefreshExpiredLogsOut`.
   - Make the **second `Do` (the refresh call) return a clean `401`** (`resp.SetStatusCode(StatusUnauthorized)`, `nil` error) instead of `"broken pipe"`.
   - Assert cookies **are** deleted (existing assertions at 168–169 stay) and client status is 401.

2. **New** `TestForwardRequestWithAuthRefreshTransientKeepsSession`:
   - First `Do` (original request) → 401.
   - Second `Do` (refresh) → `return fmt.Errorf("broken pipe")` (transport error).
   - Assert: **no `Set-Cookie` deletion** — `ctx.Response.Header.PeekCookie(...)` must NOT contain `name=;` for either cookie.
   - Assert: client status `503`.
   - Assert: `ForwardRequest` returns `nil` (handled, not a hard error).

3. **New** `TestForwardRequestWithAuthRefreshApi5xxKeepsSession`:
   - Second `Do` (refresh) → `resp.SetStatusCode(StatusInternalServerError)`, `nil` error (exercises the non-200/non-401 branch in `refresh_token.go:78-85`, which returns `err != nil`).
   - Same assertions as #2 (cookies preserved, `503`).

4. Review `refresh_token_test.go` for any case asserting the old logout-on-error contract; align expectations.

Run: `make test` (`go test -race -v ./...`). Regenerate mocks only if interfaces change (`make mock-gen`) — this change doesn't alter interfaces.

## Risks & notes

- **CORS on the `503` path**: CORS headers are applied by gateway middleware on `ctx.Response`, not copied from the upstream. Confirm the browser can still read the `503` (ACAO present) — add an assertion or verify manually with a cross-origin request.
- **Behaviour change for clients** relying on the old "401 on any refresh failure": only the gateway's own SPA consumes this; the new `503` is strictly better for it.
- **No interface/signature changes** → no mock regeneration, no OpenAPI change (the generic proxy already returns arbitrary upstream statuses; `503` is within that contract).
- Keep the existing success and CSRF-seed paths untouched — this change only re-routes the two empty-`Auth` cases.

## Rollout

1. Edit `service/api/forward.go` as above.
2. Update/add the three tests; run `make test`.
3. `make run` locally; exercise: valid session + API returning `5xx` on `/auth/refresh` ⇒ cookies survive, client gets `503`; expired refresh token ⇒ cookies cleared, client gets `401`.
4. Deploy via the standard gateway pipeline.
