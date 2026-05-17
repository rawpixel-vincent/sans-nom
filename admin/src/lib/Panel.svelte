<script lang="ts">
  export let title: string;
</script>

<section class="rounded border border-slate-300 bg-white shadow-sm">
  <header class="border-b border-slate-200 px-3 py-2">
    <h3 class="text-sm font-semibold uppercase tracking-wide text-slate-600">{title}</h3>
  </header>
  <div class="p-3">
    <slot />
  </div>
</section>
