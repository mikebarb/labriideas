<script>
  import BrushStroke from './BrushStroke.svelte';
  
  let {
    title = "L'Abri Ideas Library",
    subtitle = "",
    backgroundImage = import.meta.env.BASE_URL + "images/labri-home.avif",
    showBrushStroke = true,
    height = "h-[500px]",
    blurBackground = false,
    isSerif = false,
    color = "white"
  } = $props();
</script>

<div class="relative w-full {height} flex items-center justify-center overflow-hidden">
  <img 
    src={backgroundImage} 
    alt={title}
    class="absolute inset-0 w-full h-full object-cover"
    class:blur-md={blurBackground}
  />
  
  <!-- <div class="absolute inset-0 bg-black/40"></div> -->

  <div class="relative z-10 inline-flex flex-col items-center justify-center p-8 md:p-12 overflow-visible">

    {#if showBrushStroke}
      <div class="absolute translate-x-8 -translate-y-22 w-75 h-50 z-0">
        <BrushStroke 
          color="white"
          className="absolute inset-0 w-full h-full opacity-70 z-0 scale-[2.5] md:scale-[3]" 
        />
      </div>
    {/if}

    <h1 
      class="relative z-10 text-4xl md:text-6xl text-center tracking-[0.5em] font-light text-{color} {isSerif ? 'font-serif' : `font-sans`}"
    >
      {@html title}
    </h1>

    {#if subtitle}
      <h2 
        class="relative z-10 text-4xl md:text-7xl text-center tracking-[0.35em] font-sans font-medium text-{color} {isSerif ? 'font-serif' : `font-sans`}"
      >
        {@html subtitle}
      </h2>
    {/if}
  </div>
</div>
