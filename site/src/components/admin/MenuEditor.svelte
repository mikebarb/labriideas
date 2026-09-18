<!-- src/components/admin/MenuEditor.svelte -->
<script lang="ts">
  // Menu.json editor — Step B of the GitOps menu pipeline.
  //
  // NEW in Step B (vs the Step A textarea scaffold):
  //   - CodeMirror 6 editor surface, LAZY-LOADED via dynamic import —
  //     the editor chunk (CodeMirror + JSON language + schema tooling)
  //     is never part of the public bundle; it downloads only when an
  //     admin opens this page.
  //   - Schema-aware validation: menu.schema.json is fetched from the
  //     Go server (GET /api/schema/menu — the single source of truth,
  //     per design). codemirror-json-schema wires it into the editor
  //     as red squiggle diagnostics + autocompletion + hover docs.
  //   - The preview loop: every save/revert/discard notifies
  //     menuDataStore, so the admin sees the draft live in MegaMenu /
  //     TopicsTree (logged-in admins only) without leaving the editor.
  //
  // Still ahead (Step C): "Save & Deploy" → POST /api/update-menu →
  // GitHub commit → CI/CD rebuild → draft auto-clear.
  //
  // Schema availability is GRACEFUL: until the Go endpoint deploys (or
  // if offline), the editor falls back to parse-only validation —
  // exactly the Step A behavior. Nothing breaks, the schema features
  // simply light up once the endpoint exists.

  import { onMount, onDestroy, tick } from 'svelte';
  import masterMenu from '../../data/menu.json';
  import {
    loadMenuDraft,
    saveMenuDraft,
    clearMenuDraft,
  } from '../../lib/menuDraftStore';
  import { applyMenuDraft, revertMenuToMaster } from '../../lib/menuDataStore';
  import { authClient } from '../../lib/authClient';

  // ─── Editor plumbing (typed `any` deliberately) ───
  // DOM ref for the CodeMirror host. $state so Svelte 5 tracks the
  // bind:this assignment without warning; it's assigned once when the
  // div mounts and read only inside onMount afterwards.
  let editorHost = $state<HTMLDivElement | null>(null);
  let view: any = null;             // EditorView instance
  let lintStateFacet: any = null;   // @codemirror/lint facet, for reading diagnostics
  let editorFailed = $state(false);
  let editorReady = $state(false);

  // ─── Editor state (carried over from Step A) ───
  let loading = $state(true);
  let editorText = $state('');
  let draftLoaded = $state(false);
  let viewingMaster = $state(false);
  let dirty = $state(false);
  let parseError = $state('');
  let schemaErrorCount = $state(0);   // count of schema diagnostics (severity=error)
  let schemaAvailable = $state(false);
  let saveMessage = $state('');
  let lastSavedAt = $state('');
  let busy = $state(false);
  let deploying = $state(false);

  let diagnosticsTimer: ReturnType<typeof setTimeout> | undefined;
  onDestroy(() => clearTimeout(diagnosticsTimer));

  const API_BASE = import.meta.env.PUBLIC_API_BASE_URL;

  async function handleCommitAndDeploy() {
    deploying = true;
    saveMessage = 'Committing and triggering build...';

    try {
      const response = await authClient.fetch(`${API_BASE}/api/update-menu`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: editorText, // The validated JSON string
      });

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || 'Failed to commit changes');
      }

      // Success: The Go server has committed the file.
      // Clear the draft to reset the preview loop.
      await clearMenuDraft();
      
      saveMessage = '✅ Successfully deployed! Refreshing...';
      setTimeout(() => window.location.reload(), 2000);

    } catch (err) {
      console.error('Deployment failed:', err);
      saveMessage = '❌ Deployment failed: ' + (err as Error).message;
    } finally {
      deploying = false;
    }
  }

  // ─── Validation ───
  // Parse check stays synchronous and authoritative for syntax — cheap,
  // immediate, and independent of the editor's async lint cycle.
  function validateJson(text: string): string {
    if (!text.trim()) return 'The menu cannot be empty.';
    try {
      JSON.parse(text);
      return '';
    } catch (e) {
      return e instanceof Error ? e.message : 'Invalid JSON.';
    }
  }

  // Reads CodeMirror's lint diagnostics (schema violations, once the
  // schema extension is active). Debounced by scheduleDiagnostics()
  // because the linter runs asynchronously ~750ms after a doc change.
  // Best-effort by design: if the facet isn't reachable, gating falls
  // back to parse-only and the schema issues remain visible in-editor
  // as squiggles. The authoritative schema gate is the SERVER (Step C).
  function refreshDiagnostics() {
    if (!view || !lintStateFacet) return;
    try {
      const lint = view.state.facet(lintStateFacet);
      const diags = lint ? (lint.diagnostics as any[]) : [];
      schemaErrorCount = diags.filter(d => d.severity === 'error').length;
    } catch {
      // Advisory only — never let diagnostics reading break editing.
    }
  }

  function scheduleDiagnostics() {
    clearTimeout(diagnosticsTimer);
    diagnosticsTimer = setTimeout(refreshDiagnostics, 900);
  }

  // ─── Init ───
  onMount(async () => {
    // ── 1. Starting point: existing draft, else the deployed master ──
    try {
      const draft = await loadMenuDraft();
      if (draft) {
        editorText = JSON.stringify(draft.menu, null, 2);
        draftLoaded = true;
        viewingMaster = false;
        lastSavedAt = new Date(draft.savedAt).toLocaleString();
        saveMessage = 'Resumed from your saved draft.';
      } else {
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
      saveMessage = 'Draft store unavailable — started from master.';
    }

    // ── 2. Lazy-load the CodeMirror chunk (code-splitting: only this
    //        admin page ever fetches it) ──
    let EditorView: any, basicSetup: any, json: any, jsonSchema: any;
    try {
      const cm = await import('codemirror');
      EditorView = cm.EditorView;
      basicSetup = cm.basicSetup;
      const jsonLang = await import('@codemirror/lang-json');
      json = jsonLang.json;
      const cmSchema = await import('codemirror-json-schema');
      jsonSchema = cmSchema.jsonSchema;
      const lint = await import('@codemirror/lint');
      lintStateFacet = (lint as any).lintState ?? null;
    } catch (err) {
      console.error('MenuEditor: CodeMirror chunk failed to load', err);
      editorFailed = true;
      loading = false;
      return;
    }

    // ── 3. Schema fetch (graceful degradation — 404/offline is expected
    //        until the Step C deploy adds the endpoint) ──
    let schema: any = null;
    try {
      const res = await fetch(`${API_BASE}/api/schema/menu`);
      if (res.ok) {
        schema = await res.json();
        schemaAvailable = true;
      }
    } catch {
      // Offline or endpoint absent — schema features disabled.
    }

    // ── 4. Reveal the host div, flush the DOM, THEN mount CodeMirror ──
    // THE FIX for "editor host element missing": the host div lives in
    // the {:else} branch of the template conditional — it does not exist
    // in the DOM while loading=true. Any guard or EditorView construction
    // that runs before this point sees editorHost === null BY DESIGN.
    // Flip loading false, await tick() so Svelte writes the div to the
    // DOM (bind:this assigns), and only then construct the editor.
    loading = false;
    await tick();

    if (!editorHost) {
      console.error('MenuEditor: editor host element missing after render');
      editorFailed = true;
      return;
    }

    // ── 5. Build and mount the editor ──
    const extensions: any[] = [
      basicSetup,
      json(),
      EditorView.lineWrapping,
      // Force the editor to use a light theme so text remains dark
      EditorView.theme({
        "&": { backgroundColor: "white", color: "#1e293b" },
        ".cm-content": { caretColor: "#000" },
        "&.cm-focused .cm-cursor": { borderLeftColor: "#000" }
      }),
      EditorView.updateListener.of((u: any) => {
        if (u.docChanged) {
          editorText = u.state.doc.toString();
          dirty = true;
          saveMessage = '';
          parseError = validateJson(editorText);
          scheduleDiagnostics();
        }
      }),
    ];
    if (schema) {
      // Schema-driven validation (diagnostics), autocompletion, hover.
      extensions.push(jsonSchema(schema));
    }

    view = new EditorView({ doc: editorText, extensions, parent: editorHost });
    editorReady = true;
    refreshDiagnostics(); // catch diagnostics from the initial doc
  });

  // Programmatic doc replacement (revert/discard handlers). Dispatch is
  // synchronous, so the updateListener fires during the call — callers
  // set their final dirty/parseError state AFTER this returns.
  function setEditorText(text: string) {
    if (view) {
      view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: text } });
    } else {
      editorText = text;
    }
  }

  // ─── Handlers ───
  async function handleSaveDraft() {
    // Gate 1: parse validity (the draft store must only ever hold
    // valid JSON — the preview components depend on it).
    const err = validateJson(editorText);
    if (err) {
      parseError = err;
      saveMessage = '';
      return;
    }
    // Gate 2: schema validity — only enforced when the schema is
    // actually loaded (see refreshDiagnostics for the best-effort
    // caveat; the server re-validates authoritatively in Step C).
    if (schemaAvailable && schemaErrorCount > 0) {
      saveMessage = '';
      return;
    }
    // Overwrite guard: viewing MASTER with a saved DRAFT — saving here
    // would silently replace that draft.
    if (viewingMaster && draftLoaded) {
      const ok = confirm(
        'You are viewing the master, but you have a saved draft. ' +
        'Saving now will REPLACE that draft with this version. Continue?'
      );
      if (!ok) return;
    }
    busy = true;
    try {
      const parsed = JSON.parse(editorText);
      await saveMenuDraft(parsed);
      dirty = false;
      draftLoaded = true;
      viewingMaster = false;
      lastSavedAt = new Date().toLocaleString();
      // PREVIEW: push the saved draft into the live menu store —
      // MegaMenu/TopicsTree re-render for this admin immediately.
      applyMenuDraft(parsed);
      saveMessage = `Draft saved at ${lastSavedAt}. Preview is live while you browse.`;
    } catch (e) {
      console.error('MenuEditor: failed to save draft', e);
      saveMessage = '';
      parseError = 'Could not save the draft (IndexedDB error).';
    } finally {
      busy = false;
    }
  }

  async function handleRevertToMaster() {
    if (dirty && !confirm('Discard your unsaved edits and view the deployed master? (Your saved draft is kept.)')) {
      return;
    }
    setEditorText(JSON.stringify(masterMenu, null, 2));
    viewingMaster = true;
    dirty = false;
    parseError = '';
    schemaErrorCount = 0;
    // PREVIEW: the editor shows master, so the live menu reverts too.
    revertMenuToMaster();
    saveMessage = 'Viewing the deployed master. Your saved draft is unchanged.';
  }

  async function handleRevertToDraft() {
    if (dirty && !confirm('Discard your unsaved edits and reload your saved draft?')) {
      return;
    }
    busy = true;
    try {
      const draft = await loadMenuDraft();
      if (!draft) {
        saveMessage = 'No saved draft found — still viewing the master.';
        draftLoaded = false;
        return;
      }
      setEditorText(JSON.stringify(draft.menu, null, 2));
      viewingMaster = false;
      dirty = false;
      parseError = validateJson(editorText);
      lastSavedAt = new Date(draft.savedAt).toLocaleString();
      // PREVIEW: draft is on screen — push it live again.
      applyMenuDraft(draft.menu);
      scheduleDiagnostics();
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
      setEditorText(JSON.stringify(masterMenu, null, 2));
      draftLoaded = false;
      viewingMaster = true;
      dirty = false;
      parseError = '';
      schemaErrorCount = 0;
      lastSavedAt = '';
      // PREVIEW: draft deleted — master must be live again.
      revertMenuToMaster();
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
      <p class="text-slate-400 text-sm">Loading editor...</p>
    {:else if editorFailed}
      <p class="text-sm text-red-400">
        ❌ The editor failed to load (network/chunk error). Refresh the page to retry.
      </p>
    {:else}
      <!-- CodeMirror mounts into this host div. Styling mirrors the Step A
           textarea so the visual language is continuous. -->
      
       <!-- CHANGED: Swapped from dark-slate (low contrast) to high-contrast white -->
      <div
        bind:this={editorHost}
        class="w-full h-[60vh] overflow-y-auto font-mono text-sm leading-relaxed bg-white border border-slate-300 rounded p-1 text-slate-900 focus-within:border-cyan-500"
      ></div>
      <!-- OLD: Original dark-slate styling 
      <div
        bind:this={editorHost}
        class="w-full h-[60vh] overflow-y-auto font-mono text-xs leading-relaxed bg-slate-950 border border-slate-600 rounded text-slate-200 focus-within:border-cyan-500"
      ></div>
      -->
      
      <!-- Validation status line: parse errors first (authoritative,
           instant), then schema diagnostics (when the schema is live). -->
      {#if parseError}
        <p class="mt-3 text-sm font-medium text-red-400">❌ Invalid JSON: {parseError}</p>
      {:else if schemaAvailable && schemaErrorCount > 0}
        <p class="mt-3 text-sm font-medium text-red-400">
          ❌ {schemaErrorCount} schema violation{schemaErrorCount === 1 ? '' : 's'} — see the underlined errors in the editor.
        </p>
      {:else if !schemaAvailable}
        <p class="mt-3 text-sm text-emerald-400">✓ Valid JSON <span class="text-slate-500">(schema validation unavailable — endpoint offline or not yet deployed)</span></p>
      {:else}
        <p class="mt-3 text-sm text-emerald-400">✓ Valid JSON — passes the menu schema</p>
      {/if}

      <div class="mt-4 flex flex-wrap gap-3">
        <button
          onclick={handleSaveDraft}
          disabled={busy || !!parseError || !dirty || (schemaAvailable && schemaErrorCount > 0)}
          class="flex-1 min-w-40 bg-cyan-500 hover:bg-cyan-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
          {busy ? 'Saving...' : 'Save Draft'}
        </button>

            <!-- NEW: Deploy button -->
        <button
          type="button"
          onclick={handleCommitAndDeploy}
          disabled={deploying || !!parseError}
          class="bg-orange-600 hover:bg-orange-700 text-white font-bold px-4 py-2 rounded transition-colors disabled:opacity-50"
        >
          {deploying ? 'Deploying...' : 'Save & Deploy'}
        </button>


        {#if viewingMaster && draftLoaded}
          <button
            onclick={handleRevertToDraft}
            disabled={busy}
            class="flex-1 min-w-40 bg-amber-500 hover:bg-amber-400 disabled:opacity-60 disabled:cursor-not-allowed text-slate-900 font-bold py-2 px-4 rounded transition-colors">
            Revert to Draft
          </button>
        {:else}
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
        <!-- STEP C lands here: "Save & Deploy" → POST /api/update-menu
             → GitHub commit → CI/CD → clearMenuDraft() on success. -->
      </div>

      {#if saveMessage}
        <p class="mt-4 text-center text-sm text-slate-300">{saveMessage}</p>
      {/if}
    {/if}
  </div>
</div>
