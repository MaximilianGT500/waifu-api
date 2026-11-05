---
sidebar_position: 4
sidebar_label: Rate Limiting
title: Rate Limiting
---

# Rate Limiting

The API enforces per-IP rate limits to ensure fair usage for all consumers
(new in the Go backend; the limits are configurable by the API administrator).

## Limits

- **Rate**: `RATE_LIMIT_REQUESTS_PER_MINUTE` requests per minute (default: **240**).
- **Burst**: `RATE_LIMIT_BURST` excess requests beyond the base rate (default: **40**).
- **Login**: `POST /auth/discord` has a stricter limit (default: **10** per minute, burst **5**).
- **Uploads**: `POST /images/upload` has its own limit (default: **20** per minute, burst **10**).
- **Rejection**: When the burst buffer is full, the API responds with HTTP `429 Too Many Requests`.
- **`Retry-After` header**: Included in every `429` response. It indicates how many seconds to wait before retrying.

Limits apply per client IP address. IPv6 clients are grouped per `/64` network, because a single
connection usually owns the whole prefix.

## Rate Limit Headers

Every response that passed the rate limiter carries:

| Header | Meaning |
|--------|---------|
| `X-RateLimit-Limit` | Requests per minute allowed for this endpoint group |
| `X-RateLimit-Remaining` | Requests left before the limiter starts rejecting (burst included) |
| `Retry-After` | Only on `429`: seconds until the next request is accepted |

Use `X-RateLimit-Remaining` to slow down proactively instead of waiting for a `429`.

## Request Limits

To keep the API fast for everyone, some request parameters are bounded:

- `pageSize` is capped (default maximum: **500**); larger values are clamped.
- Multi-value filters such as `includedTags`, `excludedTags`, `includedArtists` or `includedIds` accept at most
  **50** values per parameter (`400 Bad Request` otherwise). Split larger lookups into several requests.

## Handling Rate Limits

When you receive a `429` response, check the `Retry-After` header. It indicates how many seconds to wait before retrying.

### Example: Exponential Backoff

```python
import time
import requests

def fetch_with_backoff(url, max_retries=5):
    for attempt in range(max_retries):
        response = requests.get(url)
        if response.status_code != 429:
            return response
        wait = float(response.headers.get("Retry-After", 2 ** attempt))
        time.sleep(wait)
    return response
```

```javascript
async function fetchWithBackoff(url, maxRetries = 5) {
  for (let attempt = 0; attempt < maxRetries; attempt++) {
    const response = await fetch(url);
    if (response.status !== 429) return response;
    const wait = response.headers.get("Retry-After") ?? 2 ** attempt;
    await new Promise((r) => setTimeout(r, Number(wait) * 1000));
  }
}
```

## Abuse Policy

Excessive and abusive use of the API can result in being blacklisted. Reaching that threshold requires persistent and excessive violations -- normal usage and occasional traffic spikes will not cause issues.

## For API Operators

The client IP is taken from the TCP connection. Behind a reverse proxy, list the proxy addresses in
`TRUSTED_PROXIES` (default: loopback and private networks) so `X-Forwarded-For` is honoured – headers from
untrusted peers are ignored, which prevents clients from spoofing their address. Behind Cloudflare, set
`REAL_IP_HEADER=CF-Connecting-IP` and add the [Cloudflare address ranges](https://www.cloudflare.com/ips/) (or
your own reverse proxy in front of the API) to `TRUSTED_PROXIES`. The header is only used when the request
reached the stack through one of these addresses, so clients that call the origin directly cannot choose their
address. Without it, all visitors that reach the API through the same Cloudflare edge share one rate limit; the
API logs a hint once when it sees the header.

With the bundled `docker-compose.yml`, these two settings configure the gateway in front of the API instances:
it decides the client address once and forwards exactly that address, and the API instances only trust the
gateway. If clients can reach the published gateway port directly, list only your edge in `TRUSTED_PROXIES` (or
`none`); if your reverse proxy runs on the same host, publish the gateway port on `127.0.0.1` only.

With several API instances, point all of them at the same Valkey or Redis server (`REDIS_URL`; the bundled
`docker-compose.yml` does this) so every client has one budget no matter which instance answers. Without
`REDIS_URL`, each instance limits on its own. The gateway additionally limits concurrent requests per client
(`GATEWAY_MAX_CONNECTIONS_PER_CLIENT`, default 64) and accepts request bodies above 1 MB only for image uploads
and file replacements (`GATEWAY_MAX_BODY_SIZE`).
