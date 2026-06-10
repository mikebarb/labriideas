<script>
  import BrushStroke from './BrushStroke.svelte';
  export let title = "L'Abri Ideas Library";
  export let backgroundImage = import.meta.env.BASE_URL + "images/labri-home.avif";  
                            // "/images/labri-home.avif"; // Update path to your actual image
</script>

<div class="relative w-full h-[400px] flex items-center justify-center overflow-hidden">
  <img 
    src={backgroundImage} 
    alt="L'Abri Library Bookshelf" 
    class="absolute inset-0 w-full h-full object-cover"
  />
  
  <!-- Dark overlay to ensure text legibility -->
  <div class="absolute inset-0 bg-black/40"></div>

 <!-- Container for the overlay effect -->
  <!-- Added inline-flex so the container shrinks to fit the text width -->
  <div class="relative z-10 inline-flex items-center justify-center p-8 md:p-12 overflow-visible">

    <!-- The Brushstroke (absolute) -->
     <!-- The Brushstroke: scale makes it larger than the text box -->
    <div class="absolute translate-x-8 -translate-y-22 w-[300px] h-[200px] z-0">
      <BrushStroke 
        color="white"
        className="absolute inset-0 w-full h-full opacity-70 z-0 scale-[2.5] md:scale-[3]" 
      />
    </div>
  <!-- Title -->
  <!-- <div class="relative z-10 p-6 border-2 border-white bg-white/10 backdrop-blur-sm"> 
    <h1 class="text-white text-4xl md:text-5xl font-bold uppercase tracking-wider text-center"> 
    <h1 class="relative z-20 text-black text-4xl md:text-5xl font-bold uppercase tracking-wider text-center"> -->
     <!-- Title: z-10 ensures it sits above the z-0 brushstroke -->
    <h1 class="relative z-10 font-serif italic text-slate-900 text-4xl md:text-6xl font-bold text-center drop-shadow-[0_2px_4px_rgba(255,255,255,0.8)]">
      {title}
    </h1>
  </div>
</div>
