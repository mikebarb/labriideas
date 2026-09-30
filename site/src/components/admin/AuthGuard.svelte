<!-- src/components/admin/AuthGuard.svelte
     Wraps admin page content (in LayoutAdmin). Redirects unauthenticated
     visitors to /admin/login, remembering the intended destination.
     This is a UX gate only — real enforcement is the Go authMiddleware.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { isAdmin, authChecked, refreshAuth } from '../../lib/appStatusStore';

  function redirectToLogin() {
    const here = window.location.pathname + window.location.search;
    // CHANGED: replace, not assign. assign() PUSHES the login page onto
    // history, leaving the guarded page behind it — so pressing Back
    // from login returns to the guarded page, this guard re-fires, and
    // the user ping-pongs admin → login → admin → login forever instead
    // of ever reaching their previous page. replace() SUBSTITUTES the
    // login entry for the guarded one: history becomes home → login,
    // so Back from login exits cleanly to home. The redirect param is
    // preserved either way, so post-login still returns the admin to
    // the page they wanted.
    window.location.replace(`/admin/login?redirect=${encodeURIComponent(here)}`);
  }

  // Auth check runner — shared by mount and the bfcache restore below.
  function checkAuth() {
    refreshAuth().then((ok) => {
      if (!ok) redirectToLogin();
    });
  }

  onMount(() => {
    checkAuth();
    window.addEventListener('pageshow', handlePageshow);
    return () => window.removeEventListener('pageshow', handlePageshow);
  });

  // NEW: bfcache self-heal. The browser's Back/Forward cache can restore
  // this page as a frozen DOM snapshot — in exactly the state it was
  // abandoned ("Checking credentials…", mid-redirect) — WITHOUT
  // re-running onMount. Nothing re-checks, nothing redirects: the admin
  // sees a dead page until a manual reload. pageshow with persisted=true
  // fires on every bfcache restore; re-run the check there.
  function handlePageshow(e: PageTransitionEvent) {
    if (e.persisted) checkAuth();
  }

  // Handles mid-session expiry: a 401 anywhere clears the token,
  // isAdmin flips false, and this sends the user back to login.
  $: if ($authChecked && !$isAdmin) redirectToLogin();
</script>

{#if $authChecked && $isAdmin}
  <slot />
{:else}
  <div class="flex min-h-screen items-center justify-center">
    <p class="text-slate-400">Checking credentials…</p>
  </div>
{/if}
