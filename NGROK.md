# Remote access via ngrok

Lexicon's Vite dev server proxies `/api` to the Go backend, so a **single ngrok tunnel pointed at port 5173** is all you need. The backend stays on `localhost:8787`.

## One-time setup

1. Sign up at <https://ngrok.com> if you haven't (free tier is fine).
2. Copy your auth token from the dashboard and run once:
   ```
   ngrok config add-authtoken <YOUR_TOKEN>
   ```

## Daily start

Three terminals, three commands:

**Terminal 1 — backend** (`c:\Users\kevin\CascadeProjects\lexicon\backend`)
```
go run ./cmd/server
```

**Terminal 2 — frontend** (`c:\Users\kevin\CascadeProjects\lexicon\frontend`)
```
npx vite --host
```

**Terminal 3 — tunnel** (any directory)
```
ngrok http 5173
```

ngrok prints a URL like `https://abc123.ngrok-free.app` — that's your public URL. Visit it from your phone, laptop, anywhere.

## Notes

- **First visit on free tier** shows an ngrok interstitial warning page. Click "Visit Site" once per device.
- **HMR works** over the tunnel because we set `clientPort: 443` in `vite.config.ts`.
- **Audio streaming** works via byte-range requests through the tunnel; expect some latency on first seek.
- **Spotify in-app playback** continues to work because the Web Playback SDK talks directly to Spotify, not through your tunnel. The 30-min history sync also runs entirely server-side.
- **Bandwidth**: ngrok free tier limits to 1 GB/month inbound. If you stream a lot, get a paid plan or self-host with Tailscale.

## Ngrok free tier limitations

### Interstitial warning page

The first time a device visits your ngrok-free URL it sees an **interstitial warning page** ("Visit Site" button). The `ngrok-skip-browser-warning: 1` header added by the API client bypasses this for `fetch` / `XMLHttpRequest` calls automatically — no user action needed.

For **browser-navigated** requests (e.g. visiting the root URL in a new tab) the header isn't sent, so the interstitial still appears on first access. Clicking "Visit Site" on any page sets a cookie that skips the interstitial for all subsequent requests.

### Audio streaming limitation

The `<audio>` element cannot set custom HTTP headers, so `ngrok-skip-browser-warning` has **no effect** on audio stream requests (`/api/stream/*`). If ngrok's free tier forces an interstitial page, audio playback may fail with a CORS or load error. Workarounds:

- **Click "Visit Site"** in a browser tab first to set the cookie before using the app.
- **Upgrade to a paid ngrok plan** (no interstitial at all).
- **Use Tailscale Funnel** (see below) for header-free streaming.

### Bandwidth limits

The ngrok free tier caps total **inbound traffic at 1 GB/month**. Streaming audio chews through this quickly (roughly 1 MB per minute of a 128 kbps stream). If you listen to more than ~15 hours of music per month, consider:

- Upgrading to a paid ngrok plan (higher limits, reserved subdomains).
- Self-hosting behind **Tailscale Funnel** (no bandwidth cap, no interstitial).

### Tailscale Funnel alternative

[Tailscale Funnel](https://tailscale.com/kb/1223/funnel/) exposes a local service to the internet through a Tailscale node, with **no interstitial page, no bandwidth cap, and no header hacks**. Setup:

1. Install Tailscale on the host machine and log in.
2. Enable Funnel in the Tailscale admin console.
3. Run:
   ```sh
   tailscale funnel 5173
   ```
All HTTP headers pass through unmodified — audio streaming, API calls, everything just works. The main trade-off: Tailscale Funnel doesn't have a free DNS wildcard like `*.ngrok-free.app`, so you either use the Tailscale node name or bring your own domain.

## Optional: reserved domain

Free ngrok URLs change every restart. With a paid plan you can reserve a stable subdomain:

```
ngrok http 5173 --domain=your-name.ngrok.app
```

## Security

- The dev server has no auth. **Anyone with your ngrok URL can browse your library and trigger DeepSeek calls.** Treat the URL as a secret.
- For tighter control, add HTTP Basic auth at the tunnel layer:
  ```
  ngrok http 5173 --basic-auth="kevin:somestrongpassword"
  ```
- Or use a Tailscale tailnet instead of ngrok for private access without exposing anything publicly.
