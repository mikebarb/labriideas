<script>
  let csvInput = `Id,Title,File Name
1,Little House,Little House v1
2,Hop On,Hop On 2026 edition
3,A Joyful Day,Joyful Day`;
  
  let jsonOutput = "";
  let fileInput; // Variable to hold a reference to the hidden HTML input element

  function convertToJson() {
     if (!csvInput.trim()) {
      jsonOutput = "Please provide CSV data.";
      return;
    }

    const lines = csvInput.trim().split('\n');
    const rows = lines.slice(1);                // Remove the header line
    
    const data = rows.map(line => {
      const [identifier, title, fileName] = line.split(',');
      return {
        identifier: identifier.trim(),
        title: title.trim(),
        fileName: fileName.trim()
      };
    });

    jsonOutput = JSON.stringify(data, null, 2);
  }
     // 1. Trigger the hidden file input when the button is clicked
  function openFilePicker() {
    fileInput.click();
  }

  // 2. Read the file when the user selects one
  function handleFileSelect(event) {
    const file = event.target.files[0];
    if (!file) return;

    const reader = new FileReader();
    
    // 3. When the file is finished loading, update our textarea variable
    reader.onload = (e) => {
      csvInput = e.target.result;
    };
    
    // 4. Read the file as plain text
    reader.readAsText(file);

    // 5. Clear the input value so that you can reselect the same file!
    event.target.value = "";
  }

  async function saveJsonFile() {
    if (!jsonOutput) {
      alert("Please convert to JSON first!");
      return;
    }
    // Create a blog, a filelike object in memory
    const blob = new Blob([jsonOutput], { type: 'application/json' });
    //Create a temporary url pointing to that blog
    const url = URL.createObjectURL(blob);
    // create a hidden <a> tag
    const a = document.createElement('a');
    a.href = url;
    a.download = 'catalog.json';     // default file name
    // click automatically / programmaticaly to trigger the download
    a.click();
    // clean up the memory
    URL.revokeObjectURL(url);
  }


</script>

<div class="bg-slate-800 p-6 rounded-lg border border-slate-700 shadow-xl">
  <h2 class="text-2xl font-bold text-cyan-400 mb-4">CSV to JSON Converter</h2>
  
  <!-- Hidden file input. We use bind:this to talk to it from our script -->
  <input bind:this={fileInput} type="file" accept=".csv,.txt,.tsv" on:change={handleFileSelect} class="hidden" />

  <div class="flex gap-4 mb-4">
    <button 
      on:click={openFilePicker}
      class="flex-1 bg-slate-600 hover:bg-slate-500 text-white font-bold py-2 rounded transition-colors"
    >
      📂 Load Local File
    </button>

    <button 
        on:click={convertToJson}
        class="flex-1 bg-cyan-500 hover:bg-cyan-400 text-slate-900 font-bold py-2 rounded transition-colors"
    >
        Convert to JSON
    </button>

    {#if jsonOutput}
        <button 
        on:click={saveJsonFile}
        class="flex-1 bg-green-600 hover:bg-green-500 text-white font-bold py-2 rounded transition-colors"
        >
        💾 Save JSON As...
        </button>
    {/if}

  </div>

  <textarea 
    bind:value={csvInput}
    class="w-full h-32 bg-slate-900 border border-slate-600 rounded p-3 text-white mb-4 focus:border-cyan-400 outline-none"
    placeholder="Paste CSV here..."
  ></textarea>

  <div class="bg-slate-950 p-4 rounded border border-slate-700 overflow-x-auto">
    <pre class="text-xs text-green-400 font-mono">{jsonOutput || "JSON result will appear here..."}</pre>
  </div>
</div>
