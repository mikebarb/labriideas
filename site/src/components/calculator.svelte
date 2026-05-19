<script>
  // 1. Define our reactive variables
  let num1 = 0;
  let num2 = 0;
  let result = 0;

  // 2. The function that runs when the button is clicked
  function addNumbers() {
    result = num1 + num2;
  }
</script>

<!-- 3. The UI styled with Tailwind -->
<div class="bg-slate-800 p-6 rounded-lg border border-slate-700 max-w-sm mx-auto">
  <h2 class="text-2xl font-bold text-cyan-400 mb-6 text-center">Svelte Calculator</h2>
  
  <div class="flex flex-col gap-4">
    <!-- Input 1 -->
    <input 
      type="number" 
      bind:value={num1} 
      class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-white focus:outline-none focus:border-cyan-400" 
      placeholder="First number"
    />

    <!-- Input 2 -->
    <input 
      type="number" 
      bind:value={num2} 
      class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-white focus:outline-none focus:border-cyan-400" 
      placeholder="Second number"
    />

    <!-- The Add Button -->
    <button 
      on:click={addNumbers} 
      class="bg-cyan-500 hover:bg-cyan-400 text-slate-900 font-bold py-2 px-4 rounded transition-colors"
    >
      Add Numbers
    </button>

    <!-- Result Field -->
    <div class="bg-slate-900 border border-slate-600 rounded px-3 py-2 text-cyan-300 font-mono text-lg">
      Result: {result}
    </div>
  </div>
</div>
