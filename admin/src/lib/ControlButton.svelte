<script lang="ts">
  export let title: string;
  export let description: string;
  export let action: () => void | Promise<void>;
  export let variant: 'default' | 'danger' = 'default';
</script>

<button
  class={variant === 'danger'
    ? 'rounded border border-red-300 bg-red-50 p-4 text-left hover:border-red-700 hover:bg-red-100'
    : 'rounded border border-slate-300 bg-slate-50 p-4 text-left hover:border-slate-900 hover:bg-white'}
  on:click={() => action()}
>
  <span class={variant === 'danger' ? 'block text-sm font-semibold text-red-900' : 'block text-sm font-semibold text-slate-900'}>{title}</span>
  <span class={variant === 'danger' ? 'mt-1 block text-xs leading-relaxed text-red-700' : 'mt-1 block text-xs leading-relaxed text-slate-600'}>{description}</span>
</button>
