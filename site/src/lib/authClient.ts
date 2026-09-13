// src/lib/authClient.ts
const TOKEN_KEY = 'admin_token';
const API_BASE = import.meta.env.PUBLIC_API_BASE_URL;




export const authClient = {
    // 1. Login
    // Exchange a password for a session token. Server response:
    // { token, expiresAt, userID }
    async login(password: string) {
        const res = await fetch(`${API_BASE}/api/auth/login`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ password })
        });
        if (!res.ok) {
            // Surface the status so callers can distinguish 401 vs 429 vs network
            throw new Error(`login failed (${res.status})`);
        }
        const data = await res.json();
        sessionStorage.setItem(TOKEN_KEY, data.token);
    return data; // { token, expiresAt, userID }
    },

    // 2. Logout
  // Revoke the session server-side and clear the local copy.
  async logout() {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (token) {
      // Fire-and-forget: even if this fails (offline, server restart),
      // clearing the local token below is what the UI reacts to.
      fetch(`${API_BASE}/api/auth/logout`, {
        method: 'POST',
        headers: { Authorization: `Bearer ${token}` },
      }).catch(() => {});
    }
        sessionStorage.removeItem(TOKEN_KEY);
    },

  // Verify the token against the server. This is the authoritative check —
  // a token in sessionStorage may be expired or invalidated by a restart.
  // On network failure, falls back to token presence (optimistic): an
  // offline admin UI is non-functional anyway, and a valid session looks
  // identical, so we don't lock out on a transient blip.
  async status(): Promise<{ isAdmin: boolean; userID: string }> {
    const token = sessionStorage.getItem(TOKEN_KEY);
    if (!token) return { isAdmin: false, userID: '' };
    try {
      const res = await fetch(`${API_BASE}/api/auth/status`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      const data = await res.json();
      if (!data.isAdmin) sessionStorage.removeItem(TOKEN_KEY);
      return { isAdmin: !!data.isAdmin, userID: data.userID || '' };
    } catch {
      return { isAdmin: true, userID: '' }; // network error — optimistic
    }
  },

  // 3. Authenticated Fetch (The wrapper)
  // Authenticated fetch wrapper. Auto-clears the token on 401 so an
  // expired session reverts the UI to public mode.
  async fetch(url: string, options: RequestInit = {}): Promise<Response> {
        const token = sessionStorage.getItem(TOKEN_KEY);
        const headers = new Headers(options.headers || {});
        
        if (token) {
            headers.set('Authorization', `Bearer ${token}`);
        }
        
        const res = await fetch(url, { ...options, headers });
        // Session expired or revoked server-side: drop the token so the
        // UI reverts to public mode. The next admin-page navigation will
        // show the login modal again.
        if (res.status === 401) {
            sessionStorage.removeItem(TOKEN_KEY);
        }
        return res;
    },

    // 4. Check if we have a token
    hasToken(): boolean {
        return !!sessionStorage.getItem(TOKEN_KEY);
    },

    // Header set for callers that can't use authClient.fetch — notably
    // XMLHttpRequest uploads, which need progress events. Keeps token
    // access encapsulated here rather than scattered across components.
    authHeader(): Record<string, string> {
      const token = sessionStorage.getItem(TOKEN_KEY);
      return token ? { Authorization: `Bearer ${token}` } : {};
    },

    // Clear the local token after a server-side rejection (401), mirroring
    // the auto-logout behaviour in fetch(). Callers should prompt re-login.
    clearToken() {
      sessionStorage.removeItem(TOKEN_KEY);
    }

};
