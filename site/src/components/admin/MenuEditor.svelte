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
  //
  // JSON FOLDING (VSCode-style expand/collapse):
  //   - basicSetup already ships the folding UI (foldGutter + keymap),
  //     but @codemirror/lang-json registers no fold points, so nothing
  //     folds by default. A custom foldService (below) walks the JSON
  //     syntax tree and makes every Object/Array foldable — letting
  //     admins collapse the huge subMenus/hierarchy blocks in menu.json
  //     (600+ lines) down to a navigable outline, exactly like VSCode.
  //   - @codemirror/language is already in the lazy editor chunk (it's
  //     a dependency of basicSetup), so this adds zero bundle cost.
  //
  // AUTO-FOLD TO OUTLINE ON OPEN:
  //   - On mount, the editor folds the VALUE of every top-level
  //     property (featuredLectures, schaefferCollection, etc.) so
  //     admins land on a clean outline of the menu's major sections
  //     and expand only the one they came to edit. Deliberately NOT
  //     foldAll() — that would fold nested regions too (and the root
  //     braces), leaving nothing visible at all.

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

      // Success: the Go server has committed the file. The Cloudflare
      // build takes a minute or more — the old unconditional reload
      // fired ~2s after the commit, ALWAYS before the new build
      // existed, so the refresh served the stale previous bundle.
      // Now: clear the draft, then poll /api/deploy-status until the
      // deployment that includes this commit is live, THEN reload.
      //
      // WHY HERE: this is the first point where the commit is CONFIRMED.
      // Clearing the draft before the POST (or inside a failure path)
      // would delete the admin's only local copy of uncommitted work;
      // clearing only on 'deployed' leaves a "draft" badge and preview
      // banners active for content that is already committed to GitHub.
      // The draft store's job ends the moment the commit lands — the
      // build is just delivery delay of already-saved content.
      await clearMenuDraft();
      saveMessage = '⏳ Committed to GitHub — waiting for the Cloudflare build...';

      // Poll every 5s, give up after 5 minutes (builds occasionally
      // queue behind others). On timeout/unknown state we fall back to
      // an honest message instead of a misleading auto-refresh.
      const deadline = Date.now() + 5 * 60 * 1000;
      let state = 'building';
      while (Date.now() < deadline) {
        await new Promise(res => setTimeout(res, 5000));
        try {
          const status = await authClient.fetch(`${API_BASE}/api/deploy-status`);
          if (status.ok) {
            const s = await status.json();
            state = s.state;
            saveMessage = `⏳ ${s.message}`;
            if (state === 'deployed') break;
            if (state === 'failed') break;
          } else {
            state = 'unknown';
            break;
          }
        } catch {
          state = 'unknown';
          break;
        }
      }

      if (state === 'deployed') {
        saveMessage = '✅ Deployed! Reloading to show the new menu...';
        setTimeout(() => window.location.reload(), 1000);
      } else if (state === 'failed') {
        saveMessage = '❌ The commit succeeded but the build on the Cloudflare server failed — check the server dashboard.';
      } else {
        saveMessage = '✅ Committed to GitHub. The build is still running — refresh the page in a minute or two.';
      }

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
    // Folding pieces from @codemirror/language — the same module
    // basicSetup already depends on, so no additional chunk is fetched.
    let foldService: any, foldInside: any, syntaxTree: any;
    let foldEffect: any, ensureSyntaxTree: any;
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

      // WHY: basicSetup includes foldGutter() and the fold keymap
      // (Ctrl/Cmd+[ and ]), but folding only works if something tells
      // CodeMirror WHERE foldable ranges are. @codemirror/lang-json
      // ships no fold points, so we supply them ourselves via a
      // foldService. The exports needed:
      //   foldService — facet we register our fold-point logic into
      //   foldInside  — helper returning the range INSIDE a node's brackets
      //   syntaxTree  — access to the parsed JSON syntax tree
      // Plus, for the auto-fold-on-open feature:
      //   foldEffect        — the StateEffect that marks a range folded
      //   ensureSyntaxTree  — forces the (async) parser to finish, so the
      //                       tree is complete on a freshly mounted doc
      const langMod = await import('@codemirror/language');
      foldService = langMod.foldService;
      foldInside = langMod.foldInside;
      syntaxTree = langMod.syntaxTree;
      foldEffect = langMod.foldEffect;
      ensureSyntaxTree = langMod.ensureSyntaxTree;
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

    // ── JSON fold service: teach CodeMirror where menu.json folds ──
    // NOTE: the foldService callback receives the EditorState DIRECTLY
    // (not the EditorView) — hence `state.doc`, never `state.state`.
    // CodeMirror calls this once per LINE (lineStart/lineEnd delimit
    // that line's text). A line is foldable if it ENDS with an opening
    // bracket — the pretty-printed shape of menu.json means every
    // `"items": [` / `"featured": [` / `"Arts": {` line matches this.
    // We then resolve the syntax-tree node at that bracket and return
    // foldInside(node), so the fold collapses the interior while both
    // brackets stay visible — same UX as VSCode's JSON folding.
    // Because it reads the syntax tree (not raw regex on the doc),
    // it stays correct after the full-document replacements that
    // setEditorText() dispatches (revert/discard handlers).
    const jsonFolding = foldService.of((state: any, lineStart: number, lineEnd: number) => {
      // Does this line end with an opening { or [? If not, nothing to fold.
      const text = state.doc.sliceString(lineStart, lineEnd);
      const m = /([\[{])\s*$/.exec(text);
      if (!m) return null;
      const openPos = lineStart + m.index;

      // Walk up the tree from just inside the bracket to find the
      // Object/Array node that STARTS at openPos, then fold its interior.
      let node = syntaxTree(state).resolveInner(openPos + 1, 1);
      while (node) {
        if ((node.name === 'Object' || node.name === 'Array') && node.from === openPos) {
          return foldInside(node);
        }
        node = node.parent;
      }
      return null;
    });

    // ── 5. Build and mount the editor ──
    const extensions: any[] = [
      basicSetup,
      json(),
      // VSCode-style folding: pairs with the foldGutter + foldKeymap
      // that basicSetup already installs. Gutter arrows (▸/▾) appear
      // next to every foldable line; Ctrl/Cmd+[ and ] fold/unfold.
      jsonFolding,
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

    // ── 6. AUTO-FOLD TO OUTLINE: collapse to top-level keys on open ──
    // Walks the root Object's direct Property children and folds each
    // one's Object/Array VALUE — a 600-line menu.json opens as:
    //     {
    //       "topics": { … },
    //       "featuredLectures": { … },
    //       "playlists": { … },
    //       "schaefferCollection": { … },
    //       "contact L'Abri": [ … ]
    //     }
    // Nested regions inside are left UNFOLDED, so expanding a section
    // shows its keys and the admin drills down one level at a time.
    // The fold ranges use the SAME foldInside() values the foldService
    // returns, so the gutter markers and folded state agree exactly.
    function foldTopLevel() {
      // ensureSyntaxTree: the JSON parser runs incrementally/async, and
      // a freshly mounted 600-line doc may not be fully parsed yet.
      // This forces parsing through end-of-doc (5s cap, effectively
      // instant for menu.json) so we walk a COMPLETE tree. Falls back
      // to whatever is parsed if even that times out.
      const tree = ensureSyntaxTree(view.state, view.state.doc.length, 5000)
        ?? syntaxTree(view.state);

      // Locate the root Object node (the outermost { ... }).
      let root = tree.resolveInner(1, 1);
      while (root && root.name !== 'Object') root = root.parent;
      if (!root) return;

      // Each direct child of the root is a Property node; its LAST
      // child is the property's value. Batch one foldEffect per
      // top-level Object/Array value into a single dispatch.
      const effects: any[] = [];
      let prop = root.firstChild;
      while (prop) {
        const value = prop.lastChild;
        if (value && (value.name === 'Object' || value.name === 'Array')) {
          const range = foldInside(value);
          if (range) effects.push(foldEffect.of(range));
        }
        prop = prop.nextSibling;
      }
      if (effects.length) {
        view.dispatch({ effects });
      }
    }
    foldTopLevel();
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

        <!-- Deploy: commits the SAVED DRAFT to GitHub via the Go
             server → Cloudflare build → reload on completion.
             GATED: only enabled when there is actually something to
             deploy — a saved draft exists (draftLoaded), it is what's on
             screen (not dirty), and it passes parse + schema validation.
             Editing the master directly requires Save Draft first —
             deploy is never a substitute for saving. -->
        <button
          type="button"
          onclick={handleCommitAndDeploy}
          disabled={deploying || !draftLoaded || dirty || !!parseError || (schemaAvailable && schemaErrorCount > 0)}
          class="bg-orange-600 hover:bg-orange-700 text-white font-bold px-4 py-2 rounded transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          {deploying ? 'Deploying...' : 'Deploy'}
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
