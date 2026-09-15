<!-- src/components/admin/MenuEditor.svelte -->
<script lang="ts">
  // Menu.json editor — Step A scaffold of the GitOps menu pipeline.
  //
  // Responsibilities in this step:
  //   - Load the starting point: existing IndexedDB draft if one exists,
  //     otherwise the build-time MASTER menu (the deployed state).
  //   - Provide the raw editing surface with live JSON.parse validation
  //     (parse errors are surfaced as you type — the long-term "show
  //     errors if the JSON format is incorrect" requirement).
  //   - Persist drafts to IndexedDB on demand (refresh/close-proof).
  //   - Revert-to-master / revert-to-draft / discard-draft controls.
  //
  // Deliberately NOT in this step:
  //   - Schema-aware validation + IDE surface (Step B: CodeMirror/Monaco,
  //     driven by /api/schema/menu served from the Go server).
  //   - The "Save & Deploy" commit pipeline (Step C: POST /api/update-menu
  //     → GitHub API commit → CI/CD rebuild → draft auto-clear).
  //
  // The master import is BUILD-TIME: it reflects what is currently
  // deployed, which is exactly the right baseline for a new draft.
  //
  // Source tracking: the editor always knows WHICH copy is on screen —
  // the saved draft or the deployed master (viewingMaster). This drives
  // the revert button's label/behavior and the overwrite guard in
  // handleSaveDraft.

  import { onMount } from 'svelte';
  import masterMenu from '../../data/menu.json';
  import {
    loadMenuDraft,
    saveMenuDraft,
    clearMenuDraft,
  } from '../../lib/menuDraftStore';

  // ─── Editor state ───
  let loading = $state(true);
  let editorText = $state('');          // the raw JSON text being edited
  let draftLoaded = $state(false);      // does a saved draft exist in IndexedDB?
  let viewingMaster = $state(false);    // is the editor showing master, not draft?
  let dirty = $state(false);            // unsaved changes present?
  let parseError = $state('');          // live JSON.parse error, '' if valid
  let saveMessage = $state('');
  let lastSavedAt = $state('');         // human-readable timestamp of last draft save
  let busy = $state(false);

  // ─── Init ───
  onMount(async () => {
    try {
      const draft = await loadMenuDraft();
      if (draft) {
        // Resume exactly where the admin left off — the core draft promise.
        editorText = JSON.stringify(draft.menu, null, 2);
        draftLoaded = true;
        viewingMaster = false;
        lastSavedAt = new Date(draft.savedAt).toLocaleString();
        saveMessage = 'Resumed from your saved draft.';
      } else {
        // First visit (or post-deploy clear): the deployed master is the
        // starting point. Pretty-printed for readable editing.
        editorText = JSON.stringify(masterMenu, null, 2);
        draftLoaded = false;
        viewingMaster = true;
        saveMessage = 'Loaded the deployed master menu.';
      }
      parseError = validateJson(editorText);
    } catch (err) {
      console.error('MenuEditor: failed to load draft', err);
      editorText = JSON.stringify(masterMenu, null, 2);
      viewingMaster = true;
      parseError = '';
      saveMessage = 'Draft store unavailable — started from master.';
    } finally {
      loading = false;
    }
  });

  // ─── Validation ───
  // Step A: parse-level validation only. Step B replaces this with
  // schema-aware validation (menu.schema.json from /api/schema/menu),
  // which reports structure/field errors, not just syntax.
  function validateJson(text: string): string {
    if (!text.trim()) return 'The menu cannot be empty.';
    try {
      JSON.parse(text);
      return '';
    } catch (e) {
      // SyntaxError messages already include position ("at position N") —
      // genuinely useful, so surface the message as-is.
      return e instanceof Error ? e.message : 'Invalid JSON.';
    }
  }

  // ─── Handlers ───
  function handleChange() {
    dirty = true;
    saveMessage = '';
    parseError = validateJson(editorText);
  }

  async function handleSaveDraft() {
    // Guard: never persist an unparseable draft — the draft store must
    // only ever contain valid JSON, or the Step-B editor and the future
    // preview integration would choke loading it.
    const err = validateJson(editorText);
    if (err) {
      parseError = err;
      saveMessage = '';
      return;
    }
    // Overwrite guard: we are viewing MASTER but a saved DRAFT exists.
    // Saving here would silently replace that draft with a master-based
    // version — the admin may have reverted to master just to look, and
    // must not lose their draft by reflexively clicking Save.
    if (viewingMaster && draftLoaded) {
      const ok = confirm(
        'You are viewing the master, but you have a saved draft. ' +
        'Saving now will REPLACE that draft with this version. Continue?'
      );
      if (!ok) return;
    }
    busy = true;
    try {
      await saveMenuDraft(JSON.parse(editorText));
      dirty = false;
      draftLoaded = true;
      viewingMaster = false;
      lastSavedAt = new Date().toLocaleString();
      saveMessage = `Draft saved at ${lastSavedAt}. It will survive page refreshes.`;
    } catch (e) {
      console.error('MenuEditor: failed to save draft', e);
      saveMessage = '';
      parseError = 'Could not save the draft (IndexedDB error).';
    } finally {
      busy = false;
    }
  }

  async function handleRevertToMaster() {
    // Puts the deployed master on screen. The saved draft is NOT touched —
    // the button flips to "Revert to Draft" so the way back is obvious.
    if (dirty && !confirm('Discard your unsaved edits and view the deployed master? (Your saved draft is kept.)')) {
      return;
    }
    editorText = JSON.stringify(masterMenu, null, 2);
    viewingMaster = true;
    dirty = false;
    parseError = '';
    saveMessage = 'Viewing the deployed master. Your saved draft is unchanged.';
  }

  async function handleRevertToDraft() {
    // Restores the saved draft onto the screen. The inverse of
    // handleRevertToMaster — the toggle the previous version lacked.
    if (dirty && !confirm('Discard your unsaved edits and reload your saved draft?')) {
      return;
    }
    busy = true;
    try {
      const draft = await loadMenuDraft();
      if (!draft) {
        // Shouldn't happen (button only shows when draftLoaded), but never
        // let a stale flag strand the user on a broken screen.
        saveMessage = 'No saved draft found — still viewing the master.';
        draftLoaded = false;
        return;
      }
      editorText = JSON.stringify(draft.menu, null, 2);
      viewingMaster = false;
      dirty = false;
      parseError = validateJson(editorText);
      lastSavedAt = new Date(draft.savedAt).toLocaleString();
      saveMessage = `Restored your draft (last saved ${lastSavedAt}).`;
    } catch (e) {
      console.error('MenuEditor: failed to load draft', e);
      saveMessage = 'Could not load the draft (IndexedDB error).';
    } finally {
      busy = false;
    }
  }

  async function handleDiscardDraft() {
    if (!confirm('Permanently delete the saved draft and start fresh from the master?')) {
      return;
    }
    busy = true;
    try {
      await clearMenuDraft();
      editorText = JSON.stringify(masterMenu, null, 2);
      draftLoaded = false;
      viewingMaster = true;
      dirty = false;
      parseError = '';
      lastSavedAt = '';
      saveMessage = 'Draft deleted. Editing from the deployed master.';
    } catch (e) {
      console.error('MenuEditor: failed to clear draft', e);
    } finally {
      busy = false;
    }
  }
</script>

<div class="w-full max-w-4xl mx-auto">
  <div class="bg-slate-800 p-6 rounded-lg border border-cyan-500/50 shadow-xl">
    <div class="flex items-start justify-between mb-4">
      <div>
        <h2 class="text-lg font-bold text-cyan-400">Menu Editor</h2>
        <p class="text-slate-400 text-xs mt-1">
          Drafts are private to this browser and persist until committed or discarded.
          {#if lastSavedAt}Last saved: {lastSavedAt}{/if}
        </p>
      </div>
      <!-- Draft status badges -->
      <div class="flex gap-2 shrink-0">
        {#if draftLoaded}
          <span class="text-xs bg-amber-500/20 text-amber-300 px-2 py-1 rounded">Draft in progress</span>
        {/if}
        {#if dirty}
          <span class="text-xs bg-orange-500/20 text-orange-300 px-2 py-1 rounded">Unsaved changes</span>
        {/if}
      </div>
    </div>

    {#if loading}
      <p class="text-slate-400 text-sm">Loading...</p>
    {:else}
      <!-- STEP A surface: plain textarea with live parse validation.
           Step B replaces ONLY this block with the schema-aware editor
           (CodeMirror/Monaco); every handler above stays as-is. -->
      <textarea
        bind:value={editorText}
        oninput={handleChange}
        spellcheck="false"
        rows="24"
        class="w-full font-mono text-xs leading-relaxed bg-slate-950 border border-slate-600 rounded p-3 text-slate-200 focus:outline-none focus:border-cyan-500 resize-y"
      ></textarea>

      {#if parseError}
        <p class="mt-3 text-sm font-medium text-red-400">❌ Invalid JSON: {parseError}</p>
      {:else}
        <p class="mt-3 text-sm text-emerald-400">✓ Valid JSON</p>
      {/if}
<!-- xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  -->
      <div class="mt-4 flex flex-wrap gap-3">
        <button
          onclick={handleSaveDraft}
          disabled={busy || !!parseError || !dirty}
          class="flex-1 min-w-40 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
          {busy ? 'Saving...' : 'Save Draft'}
        </button>

        {#if viewingMaster && draftLoaded}
          <button
            onclick={handleRevertToDraft}
            disabled={busy}
            class="flex-1 min-w-40 bg-amber-500 hover:bg-amber-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
            Revert to Draft
          </button>
        {:else}
          <!-- disabled when already viewing master with no unsaved
               edits — reverting to where you already are is a no-op, and an
               active button implies something would happen. (Still active
               while dirty: its real job is "discard unsaved edits and
               return to master".) -->
          <button
            onclick={handleRevertToMaster}
            disabled={busy || (viewingMaster && !dirty)}
            class="flex-1 min-w-40 bg-slate-700 hover:bg-slate-600 disabled:opacity-60 disabled:cursor-not-allowed text-slate-200 font-bold py-2 px-4 rounded transition-colors">
            Revert to Master
          </button>
        {/if}

        <button
          onclick={handleDiscardDraft}
          disabled={busy || !draftLoaded}
          class="flex-1 min-w-40 bg-red-900/60 hover:bg-red-800/60 disabled:opacity-40 disabled:cursor-not-allowed text-red-200 font-bold py-2 px-4 rounded transition-colors">
          Discard Draft
        </button>
      </div>

      <!-- xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx  -->

      {#if saveMessage}
        <p class="mt-4 text-center text-sm text-slate-300">{saveMessage}</p>
      {/if}
    {/if}
  </div>
</div>
