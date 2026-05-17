# Lexicon Black Screen Fix Plan

## Problem Summary
The Lexicon frontend renders a completely black screen on load. The browser console shows a fatal React error:

```
Error: useToast must be used within ToastProvider
```

When an uncaught error is thrown during React render, the entire component tree crashes and unmounts, leaving a blank/black screen.

---

## Root Cause Analysis

### Provider Nesting Order Bug in `App.tsx`

React Context is **only available to descendants** of a Provider. In the current `App.tsx`, the provider nesting is:

```tsx
<PlayerProvider>       {/* calls useToast() internally */}
  <ToastProvider>
    <DownloadProvider>  {/* calls useToast() internally */}
      ...
    </DownloadProvider>
  </ToastProvider>
</PlayerProvider>
```

### Why This Fails

**`PlayerContext.tsx` line 72** calls `useToast()` directly inside the `PlayerProvider` component body:

```tsx
export function PlayerProvider({ children }: { children: ReactNode }) {
  const toast = useToast();   // <-- CRASH: ToastContext is null here!
  ...
}
```

Because `PlayerProvider` is rendered **outside** `ToastProvider`, when `PlayerProvider`'s component function executes, the `ToastContext.Provider` has not yet been mounted in the tree. `useContext(ToastContext)` returns `null`, and the guard clause in `useToast()` throws:

```tsx
export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) throw new Error("useToast must be used within ToastProvider");
  return ctx;
}
```

`DownloadProvider` is **inside** `ToastProvider`, so it correctly receives the context.

### Evidence from Console Stack Trace
The stack trace points to `useToast` being called during render, confirming the context is missing at the point of consumption.

---

## Fix Plan

### Fix 1: Reorder Providers (Critical — fixes the crash)

**File:** `frontend/src/App.tsx`

Move `ToastProvider` to the **outermost** position so it is an ancestor of both `PlayerProvider` and `DownloadProvider`:

```tsx
<ToastProvider>
  <PlayerProvider>
    <DownloadProvider>
      {isMobile ? <MobileLayout /> : <DesktopLayout />}
    </DownloadProvider>
  </PlayerProvider>
</ToastProvider>
```

**Why this works:**
- `ToastProvider` now wraps both consumers.
- `PlayerProvider` and `DownloadProvider` both render as children of `ToastProvider`, so `useToast()` successfully resolves the context.

### Fix 2: Add a React Error Boundary (Defensive)

**File:** `frontend/src/components/ErrorBoundary.tsx` (new)

Create a minimal error boundary so that if a similar provider/context bug happens in the future, the user sees a readable error message instead of a black screen:

```tsx
import { Component, ReactNode } from "react";

interface Props {
  children: ReactNode;
  fallback?: ReactNode;
}

interface State {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<Props, State> {
  state: State = { hasError: false };

  static getDerivedStateFromError(error: Error): State {
    return { hasError: true, error };
  }

  render() {
    if (this.state.hasError) {
      return (
        this.props.fallback ?? (
          <div className="p-6 text-center">
            <h2 className="text-lg font-semibold text-red-400 mb-2">
              Something went wrong
            </h2>
            <pre className="text-sm text-muted bg-panel p-3 rounded overflow-auto">
              {this.state.error?.message}
            </pre>
          </div>
        )
      );
    }
    return this.props.children;
  }
}
```

**File:** `frontend/src/App.tsx`

Wrap the app content with the error boundary:

```tsx
<ToastProvider>
  <ErrorBoundary>
    <PlayerProvider>
      <DownloadProvider>
        ...
      </DownloadProvider>
    </PlayerProvider>
  </ErrorBoundary>
</ToastProvider>
```

### Fix 3: Regression Test / Verification

After applying the fix:
1. Rebuild the frontend (`npm run build` in `frontend/`).
2. Hard-refresh the browser (Ctrl+Shift+R or Cmd+Shift+R).
3. Verify:
   - No console errors.
   - The sidebar/nav and home page render correctly.
   - Toast notifications still function (e.g., trigger a download or playback error).

---

## Files Modified

| File | Change |
|------|--------|
| `frontend/src/App.tsx` | Reorder providers: ToastProvider outermost; optionally wrap with ErrorBoundary |
| `frontend/src/components/ErrorBoundary.tsx` | **New file** — optional defensive error boundary |

---

## Summary

- **Root cause:** `PlayerProvider` was accidentally placed outside `ToastProvider` during a prior bug-fix refactor, causing `useToast()` to throw on app startup.
- **Primary fix:** Swap provider nesting order so `ToastProvider` wraps both `PlayerProvider` and `DownloadProvider`.
- **Secondary fix:** Add an Error Boundary to prevent future uncaught render errors from blanking the entire screen.
- **Impact:** Single-line change in `App.tsx`; no logic changes anywhere else.
