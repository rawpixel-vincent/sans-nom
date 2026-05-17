<script lang="ts">
  import type { FileInfo } from './api';

  export let files: FileInfo[] = [];
  export let onOpen: (path: string) => void | Promise<void>;

  function formatSize(size: number) {
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    return `${(size / 1024 / 1024).toFixed(1)} MB`;
  }
</script>

<div class="max-h-[68vh] overflow-auto">
  <table class="data-table">
    <thead>
      <tr>
        <th>path</th>
        <th>size</th>
        <th>modified</th>
      </tr>
    </thead>
    <tbody>
      {#each files as file}
        <tr>
          <td>
            {#if file.is_dir}
              <span class="text-slate-500">{file.path}/</span>
            {:else}
              <button class="text-left text-slate-900 underline decoration-slate-300 hover:decoration-slate-900" on:click={() => onOpen(file.path)}>
                {file.path}
              </button>
            {/if}
          </td>
          <td class="font-mono">{file.is_dir ? '-' : formatSize(file.size)}</td>
          <td class="font-mono">{file.mod_time}</td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
