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
    window.location.assign(`/admin/login?redirect=${encodeURIComponent(here)}`);
  }

  onMount(async () => {
    const ok = await refreshAuth();
    if (!ok) redirectToLogin();
  });

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
