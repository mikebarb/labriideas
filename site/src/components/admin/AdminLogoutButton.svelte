<!-- src/components/admin/AdminLogoutButton.svelte -->
<script lang="ts">
  import { isAdmin } from '../../lib/appStatusStore';
  import { authClient } from '../../lib/authClient';
  import { refreshAuth } from '../../lib/appStatusStore';

  async function logout() {
    await authClient.logout();     // server-side revoke + clear local token
    await refreshAuth();           // isAdmin -> false
    //window.location.assign('/admin/login');
    //window.location.assign('/labri-home');   // back to the public site
    window.location.assign('/');   // back to the public site
  }
</script>

{#if $isAdmin}
  <button
    on:click={logout}
    class="rounded-lg border border-slate-600 px-3 py-1.5 text-sm text-slate-300 hover:bg-slate-800"
  >
    Sign Out
  </button>
{/if}
