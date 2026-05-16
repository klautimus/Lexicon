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
