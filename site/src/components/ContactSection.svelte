<!-- src/components/ContactSection.svelte -->
<script lang="ts">
  // Hybrid "Preview Island" (Option 2 pattern, pilot implementation).
  //
  // ROLE: renders the Contact L'Abri section with two data sources:
  //   1. The STATIC fallback — contactSection prop, extracted from the
  //      build-time menu.json by the page's frontmatter and baked into
  //      the SSR HTML. This is what public visitors and search engines
  //      receive; for them the page is byte-identical to a fully static
  //      render.
  //   2. The LIVE DRAFT — menuDataStore, but ONLY when the visitor is
  //      an admin AND a draft preview is active. The $derived switches
  //      sources, Svelte re-renders, and the admin sees their uncommitted
  //      menu.json edits on the real page.
  //
  // Non-admin behavior: the fallback prop is used forever, the store is
  // never read for rendering (only subscribed for the boolean checks),
  // and hydration cost is this one small component. Zero content flash.
  //
  // After a Step C deploy commits the draft, GitHub rebuilds the site,
  // the new master IS the static fallback, and the preview condition
  // stops triggering for everyone — the change is "baked in".

  import { menuData, menuPreviewSource } from '../lib/menuDataStore';
  import { isAdmin } from '../lib/appStatusStore';

  interface ContactDetail {
    label: string;
    value: string;
    url?: string;
    note?: string;
  }

  interface ContactSite {
    title: string;
    details: ContactDetail[];
  }

  interface ContactSectionData {
    description?: string;
    sites?: ContactSite[];
  }

  interface Props {
    // Build-time extraction from the page's frontmatter — the SSR seed
    // and the permanent fallback for non-admin visitors.
    contactSection?: ContactSectionData | null;
  }
  let { contactSection = null }: Props = $props();

  // Draft preview is live ONLY for a logged-in admin with a draft
  // applied (menuDataStore sets menuPreviewSource='draft' when both
  // conditions hold; logout or draft-clear flips it back to 'master').
  const isPreviewing = $derived($isAdmin && $menuPreviewSource === 'draft');

  // The draft's copy of this section, resolved ONCE and shared by the
  // banner, the diagnostic effect, and the source selection. undefined
  // when the draft doesn't contain the section (e.g. the subMenu key
  // was accidentally edited — the identifier is load-bearing, an
  // exact-match locator in every consumer).
  const draftSection = $derived(
    isPreviewing
      ? (($menuData as any).subMenus.find(
          (s: any) => s.subMenu === "Contact L'Abri"
        ) as ContactSectionData | undefined)
      : undefined
  );

  // Diagnostic twin of the banner state: previewing but the draft
  // lacks this section → console warn. Same condition the banner
  // renders from, so the two can never disagree.
  $effect(() => {
    if (isPreviewing && !draftSection) {
        console.warn(
          '[ContactSection] Draft preview active but "Contact L\'Abri" subMenu not found in draft — rendering the deployed master. Check the subMenu key spelling in the editor.'
        );
    }
  });

  // The single source-selection point: draft section when found, else
  // the static prop.
  const active = $derived(
    isPreviewing && draftSection ? draftSection : contactSection
  );

  const sites = $derived(active?.sites ?? []);
</script>

{#if isPreviewing && draftSection}
  <!-- Draft banner: the section on screen IS the admin's uncommitted
       draft. -->
  <div class="mb-4 px-3 py-2 bg-amber-100 border border-amber-400 text-amber-800 text-sm rounded">
    ✎ Previewing your uncommitted draft — public visitors see the deployed version.
  </div>
{:else if isPreviewing}
  <!-- Broken-preview banner: a draft IS active, but this section could
       not be found in it — the page is showing the deployed MASTER.
       Honest to the admin: matches the console warn exactly. -->
  <div class="mb-4 px-3 py-2 bg-red-100 border border-red-400 text-red-800 text-sm rounded">
    ⚠ A draft is active, but the "Contact L'Abri" section was not found in it — this page is showing the deployed master. Check the subMenu key spelling in the editor.
  </div>
{/if}

<!-- INTRO TEXT SECTION -->
<p>{active?.description}</p>

<!-- CONTACT DETAILS GRID -->
<div class="grid grid-cols-1 md:grid-cols-2 gap-x-8 gap-y-10">
  {#each sites as site (site.title)}
    <div>
      <h3 class="font-bold">{site.title}</h3>
      <p class="text-md">
        {#each site.details as detail}
          {#if detail.label && detail.label.trim() !== ''}
            <strong>{detail.label}:</strong>
          {/if}
          {' '}
          {#if detail.url}
            <a
              href={detail.url}
              target="_blank"
              rel="noopener noreferrer"
              class="underline text-md"
            >
              {detail.value}
            </a>
          {:else}
            {detail.value}
          {/if}
          {#if detail.note}
            <span class="italic text-gray-500 ml-1">{detail.note}</span>
          {/if}
          <br />
        {/each}
      </p>
    </div>
  {/each}
</div>
