<!-- src/components/OpfsTest.svelte -->
<script lang="ts">
  import { onMount } from 'svelte';
  import {
    isOpfsSupported,
    hasTrack,
    listDownloadedHashes,
    getStorageEstimate,
    formatBytes
  } from '../lib/opfsStore.ts';
  import { isOnline } from '../lib/connectivityStore.ts';

  let supported = $state(false);
  let hashes: string[] = $state([]);
  let storage = $state({ usage: 0, quota: 0 });
  let testHash = $state('test-track-123');
  let hasResult = $state<string>('Click "Check" to test');

  let tracks: string[] = $state([]);
  let testName = $state('Test-Track-123');
  let status = $state('Not Checked');

  onMount(async () => {
    supported = isOpfsSupported();
    if (supported) {
      hashes = await listDownloadedHashes();
      storage = await getStorageEstimate();

      refreshOpfs();
    }
  });

  // Reads the OPFS directory and counts/lists the files
  async function refreshOpfs() {
    try {
      const root = await navigator.storage.getDirectory();
      const dir = await root.getDirectoryHandle('labri-cache', { create: false });
      const files = [];
      // @ts-ignore - async iterator on FileSystemDirectoryHandle
      for await (const [name, handle] of dir.entries()) {
        if (handle.kind === 'file') {
          files.push(name);
        }
      }
      tracks = files;
    } catch (err) {
      // Directory doesn't exist yet - this is normal if nothing has been written
      tracks = [];
    }
  }

  // Checks if a specific file exists
  async function checkOpfs() {
    try {
      const root = await navigator.storage.getDirectory();
      const dir = await root.getDirectoryHandle('labri-cache', { create: false });
      // Throws an error if the file is not found
      await dir.getFileHandle(testName, { create: false });
      status = 'Found';
    } catch (err) {
      status = 'Not Found';
    }
  }

  // Writes a dummy text file to OPFS to prove the loop works
  async function writeMockFile() {
    try {
      const root = await navigator.storage.getDirectory();
      const dir = await root.getDirectoryHandle('labri-cache', { create: true });
      const fileHandle = await dir.getFileHandle('Test-Track-123', { create: true });
      const writable = await fileHandle.createWritable();
      // Writing a string as if it were an audio file
      await writable.write('Mock audio data blob');
      await writable.close();
      status = 'Written';
      await refreshOpfs();
    } catch (err) {
      console.error('Write failed', err);
      const message = err instanceof Error ? err.message : String(err);
      status = 'Write Error: ' + message;
    }
  }

  async function refresh() {
    hashes = await listDownloadedHashes();
    storage = await getStorageEstimate();
  }

  async function checkHash() {
    const result = await hasTrack(testHash);
    hasResult = result ? '✅ Found' : '❌ Not Found';
  }
</script>

<div class="space-y-4">
  <div class="p-4 bg-gray-100 rounded">
    <h2 class="font-semibold">Support</h2>
    <p>OPFS: {supported ? '✅ Supported' : '❌ Not Supported'}</p>
    <p>Network: {$isOnline ? '🟢 Online' : '🔴 Offline'}</p>
  </div>

  <div class="p-4 bg-gray-100 rounded">
    <h2 class="font-semibold">Storage</h2>
    <p>Used: {formatBytes(storage.usage)}</p>
    <p>Quota: {formatBytes(storage.quota)}</p>
  </div>

  <div class="p-4 bg-gray-100 rounded">
    <h2 class="font-semibold">Downloaded Tracks ({hashes.length})</h2>
    {#if hashes.length === 0}
      <p class="text-gray-500 italic">No tracks downloaded yet</p>
    {:else}
      <ul class="list-disc list-inside">
        {#each hashes as h}
          <li class="font-mono text-sm">{h}</li>
        {/each}
      </ul>
    {/if}
    <button
      onclick={refresh}
      class="mt-2 px-3 py-1 bg-blue-500 text-white rounded text-sm"
    >
      Refresh
    </button>
  </div>

  <div class="p-4 bg-gray-100 rounded">
    <h2 class="font-semibold">Test Has Check</h2>
    <div class="flex items-center gap-2">
      <input
        bind:value={testHash}
        class="border px-2 py-1 rounded"
        placeholder="hash"
      />
      <button
        onclick={checkHash}
        class="px-3 py-1 bg-green-500 text-white rounded text-sm"
      >
        Check
      </button>
    </div>
    <p class="mt-2">{hasResult}</p>
  </div>

  <div class="p-4 border rounded shadow-sm bg-white text-black">
    <h3 class="font-bold text-lg">OPFS Verification</h3>
    
    <div class="mt-4">
      <p>Downloaded Tracks: ({tracks.length})</p>
      <button onclick={refreshOpfs} class="px-2 py-1 bg-gray-200 rounded">Refresh List</button>
    </div>

    <div class="mt-4">
      <p>Test Has Check</p>
      <input bind:value={testName} class="border px-2 py-1" />
      <button onclick={checkOpfs} class="ml-2 px-2 py-1 bg-blue-500 text-white rounded">Check File</button>
      <p class="mt-2">Status: <strong>{status}</strong></p>
    </div>

    <div class="mt-4">
      <button onclick={writeMockFile} class="px-2 py-1 bg-green-600 text-white rounded">Write Mock File to OPFS</button>
    </div>
  </div>


</div>
