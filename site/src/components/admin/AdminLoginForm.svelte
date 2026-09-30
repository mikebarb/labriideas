<!-- src/components/admin/AdminLoginForm.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import { authClient } from '../../lib/authClient';
  import { refreshAuth } from '../../lib/appStatusStore';

  let password = '';
  let error = '';
  let busy = false;

  // NEW: ref to the password input for programmatic focus.
  let passwordInput: HTMLInputElement | null = null;

  // Where to go after login. SECURITY: only same-site relative paths are
  // accepted — an absolute URL or protocol-relative "//evil.com" in the
  // redirect param is rejected. Without this, the login page is an
  // open-redirect (phishing) vector.
  function safeReturnTo(): string {
    const raw = new URLSearchParams(window.location.search).get('redirect');
    if (raw && raw.startsWith('/') && !raw.startsWith('//')) return raw;
    return '/admin'; // default: admin landing — adjust if yours differs
  }

  // Already authenticated? Don't show the form — send them straight through.
  onMount(async () => {
    // NEW: focus the password field on open so the admin can simply
    // type the password and press Enter. Deliberately NOT the HTML
    // `autofocus` attribute — Svelte strips it during hydration and
    // it is an a11y antipattern; a programmatic focus after mount
    // survives hydration reliably.
    requestAnimationFrame(() => passwordInput?.focus());

    if (await refreshAuth()) window.location.assign(safeReturnTo());
  });

  // NEW: Svelte action — belt-and-braces focus after the input actually
  // renders (requestAnimationFrame covers the common case; this covers
  // any hydration edge that delays the element).
  function autofocus(node: HTMLInputElement) {
    node.focus();
    return {};
  }

  // NEW: exit without logging in. history.back() when the user arrived
  // here via in-site navigation (the common case — a guarded link
  // redirected them); fall back to the home page when the login page
  // was opened directly (new tab, bookmark — no history to go back to).
  function handleCancel() {
    if (window.history.length > 1) {
      window.history.back();
    } else {
      window.location.assign('/');
    }
  }

  function handleSubmit(event: SubmitEvent) {
    event.preventDefault();
    void handleLogin();
  }

  async function handleLogin() {
    if (busy || !password.trim()) return;
    busy = true;
    error = '';
    try {
      await authClient.login(password);
      password = ''; // don't keep the secret in memory
      window.location.assign(safeReturnTo());
    } catch (err) {
      const msg = err instanceof Error ? err.message : '';
      if (msg.includes('429')) {
        error = 'Too many attempts. Wait a few minutes and try again.';
      } else if (msg.includes('fetch') || !navigator.onLine) {
        error = 'Cannot reach the server. Check your connection.';
      } else {
        error = 'Incorrect password.';
      }
      busy = false;
    }
  }
</script>

<form
  onsubmit={handleSubmit}
  class="w-full max-w-sm rounded-2xl border border-slate-700 bg-slate-800 p-8 shadow-2xl"
>
  <h1 class="mb-2 text-center text-2xl font-bold text-white">Administrator Sign In</h1>
  <p class="mb-6 text-center text-sm text-slate-400">
    Enter your password to access the admin console.
  </p>

  <label class="mb-2 block text-sm font-medium text-slate-300" for="password">
    Password
  </label>
  <input
    id="password"
    type="password"
    bind:this={passwordInput}
    use:autofocus
    bind:value={password}
    placeholder="••••••••"
    autocomplete="current-password"
    disabled={busy}
    class="mb-4 w-full rounded-lg border border-slate-600 bg-slate-700 px-4 py-3
           text-white placeholder-slate-500 focus:border-sky-500 focus:outline-none"
  />

  {#if error}
    <p class="mb-4 rounded-lg border border-red-500/40 bg-red-500/10 px-4 py-3 text-sm text-red-300">
      {error}
    </p>
  {/if}

  <button
    type="submit"
    disabled={busy || !password.trim()}
    class="w-full rounded-lg bg-sky-600 px-4 py-3 font-semibold text-white transition-colors
           hover:bg-sky-500 disabled:cursor-not-allowed disabled:opacity-50"
  >
    {busy ? 'Signing in…' : 'Sign In'}
  </button>

  <!-- NEW: cancel/exit — same destination the browser Back button
       would take, but discoverable. type="button" so it never submits
       the form. -->
  <button
    type="button"
    onclick={handleCancel}
    disabled={busy}
    class="mt-3 w-full rounded-lg px-4 py-2 text-sm font-medium text-slate-400
           transition-colors hover:text-slate-200 disabled:cursor-not-allowed
           disabled:opacity-50"
  >
    Cancel
  </button>
</form>
