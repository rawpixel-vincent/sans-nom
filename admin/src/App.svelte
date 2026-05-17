<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import ControlButton from "./lib/ControlButton.svelte";
  import FileTable from "./lib/FileTable.svelte";
  import Panel from "./lib/Panel.svelte";
  import {
    createStory,
    getFile,
    getIndex,
    getLogs,
    getModelRuns,
    getStatus,
    getStory,
    listLorePacks,
    listStories,
    runControl,
    type AdminStatus,
    type FileInfo,
    type IndexResponse,
    type LogEvent,
    type LorePack,
    type StoryDetail,
    type StorySummary,
  } from "./lib/api";

  const tabs = [
    "overview",
    "logs",
    "model-runs",
    "index",
    "files",
    "external",
    "control",
  ];
  const commonFiles = [
    "state.json",
    "bible.json",
    "turns.jsonl",
    "index/manifest.json",
    "logs/events.jsonl",
  ];

  let status: AdminStatus | null = null;
  let stories: StorySummary[] = [];
  let lorePacks: LorePack[] = [];
  let newStoryPackID = "metabarons";
  let selectedStoryId = "";
  let detail: StoryDetail | null = null;
  let logs: LogEvent[] = [];
  let selectedLog: LogEvent | null = null;
  let categories: string[] = [];
  let modelRuns: FileInfo[] = [];
  let indexData: IndexResponse | null = null;
  let filePath = "state.json";
  let fileText = "";
  let activeTab = "overview";
  let logCategory = "";
  let logLevel = "";
  let logTail = 200;
  let logPolling = true;
  let busy = false;
  let error = "";
  let notice = "";
  let pollTimer: number | undefined;

  onMount(() => {
    void refreshAll();
    pollTimer = window.setInterval(() => {
      if (logPolling && activeTab === "logs" && selectedStoryId) {
        void loadLogsQuietly();
      }
    }, 3000);
  });

  onDestroy(() => {
    if (pollTimer !== undefined) {
      window.clearInterval(pollTimer);
    }
  });

  async function refreshAll() {
    await withBusy(async () => {
      status = await getStatus();
      lorePacks = await listLorePacks();
      if (!lorePacks.some((pack) => pack.id === newStoryPackID)) {
        newStoryPackID = lorePacks[0]?.id ?? "metabarons";
      }
      stories = await listStories();
      if (!selectedStoryId && stories?.length > 0) {
        selectedStoryId = stories[0].story_id;
      }
      if (selectedStoryId) {
        await loadStory(selectedStoryId);
      }
    });
  }

  async function loadStory(storyId: string) {
    selectedStoryId = storyId;
    detail = await getStory(storyId);
    await Promise.all([
      loadLogs(),
      loadModelRuns(),
      loadIndex(),
      loadFile(filePath),
    ]);
  }

  async function loadLogs() {
    if (!selectedStoryId) return;
    const data = await getLogs(selectedStoryId, {
      category: logCategory,
      level: logLevel,
      tail: logTail,
    });
    logs = data.events;
    categories = data.categories;
    const currentLog = selectedLog;
    if (currentLog && !logs.some((entry) => logKey(entry) === logKey(currentLog))) {
      selectedLog = null;
    }
  }

  async function loadLogsQuietly() {
    try {
      await loadLogs();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function loadModelRuns() {
    if (!selectedStoryId) return;
    modelRuns = await getModelRuns(selectedStoryId);
  }

  async function loadIndex() {
    if (!selectedStoryId) return;
    indexData = await getIndex(selectedStoryId);
  }

  async function loadFile(path: string) {
    if (!selectedStoryId) return;
    filePath = path;
    fileText = await getFile(selectedStoryId, path);
  }

  async function control(action: string, body: Record<string, unknown> = {}) {
    if (!selectedStoryId) return;
    if (!confirm(`Run ${action} for ${selectedStoryId}?`)) return;
    await withBusy(async () => {
      const result = await runControl(selectedStoryId, action, body);
      notice = `${action}: ${JSON.stringify(result)}`;
      await loadStory(selectedStoryId);
    });
  }

  async function deleteSelectedStory() {
    if (!selectedStoryId) return;
    const storyId = selectedStoryId;
    const typed = prompt(
      `Type ${storyId} to permanently delete local data, story Qdrant collection, and Honcho workspace.`,
    );
    if (typed !== storyId) return;
    if (!confirm(`Delete ${storyId} permanently? This cannot be undone.`))
      return;
    await withBusy(async () => {
      const result = await runControl(storyId, "delete-story", {
        story_id_confirmation: typed,
      });
      status = await getStatus();
      stories = await listStories();
      selectedStoryId = stories[0]?.story_id ?? "";
      detail = null;
      logs = [];
      selectedLog = null;
      modelRuns = [];
      indexData = null;
      fileText = "";
      activeTab = "overview";
      if (selectedStoryId) {
        await loadStory(selectedStoryId);
      }
      notice = `delete-story: ${JSON.stringify(result)}`;
    });
  }

  async function newStory() {
    const pack = lorePacks.find((item) => item.id === newStoryPackID);
    if (!pack) {
      error = "Select a ground lore pack first.";
      return;
    }
    await withBusy(async () => {
      const result = await createStory(pack.id);
      status = await getStatus();
      stories = await listStories();
      selectedStoryId = result.story_id;
      await loadStory(selectedStoryId);
      notice = `new story: ${result.story_id}`;
    });
  }

  async function withBusy(fn: () => Promise<void>) {
    busy = true;
    error = "";
    notice = "";
    try {
      await fn();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    } finally {
      busy = false;
    }
  }

  function pretty(value: unknown) {
    return JSON.stringify(value, null, 2);
  }

  function short(value: string, max = 72) {
    if (!value) return "";
    return value.length <= max ? value : `${value.slice(0, max - 3)}...`;
  }

  function logKey(entry: LogEvent) {
    return [
      entry.time,
      entry.level,
      entry.category,
      entry.event,
      entry.packet_id ?? "",
      entry.node_id ?? "",
      entry.run_id ?? "",
    ].join("|");
  }
</script>

<div class="min-h-screen bg-slate-100 text-slate-950">
  <header class="border-b border-slate-300 bg-white">
    <div class="flex items-center justify-between px-5 py-3">
      <div>
        <h1 class="text-lg font-semibold tracking-wide">Sans Nom Admin</h1>
        <p class="text-xs text-slate-500">
          local runtime observability and control
        </p>
      </div>
      <div class="flex items-center gap-2 text-xs">
        <span
          class:status-ok={status?.ok}
          class:status-bad={!status?.ok}
          class="rounded px-2 py-1"
        >
          bridge {status?.ok ? "ok" : "down"}
        </span>
        <button class="btn" disabled={busy} on:click={() => refreshAll()}
          >Refresh</button
        >
      </div>
    </div>
  </header>

  <main class="grid min-h-[calc(100vh-57px)] grid-cols-[320px_1fr]">
    <aside class="border-r border-slate-300 bg-slate-50 p-3">
      <div class="mb-4 rounded border border-slate-300 bg-white p-3">
        <div class="mb-2 text-xs font-semibold uppercase tracking-wide text-slate-600">
          New Story
        </div>
        <div class="flex gap-2">
          <select class="input min-w-0 flex-1" bind:value={newStoryPackID}>
            {#each lorePacks as pack}
              <option value={pack.id}>{pack.label}</option>
            {/each}
          </select>
          <button class="btn" disabled={busy || lorePacks.length === 0} on:click={() => newStory()}>
            Create
          </button>
        </div>
        {#if lorePacks.length > 0}
          <p class="mt-2 text-xs leading-relaxed text-slate-500">
            {lorePacks.find((pack) => pack.id === newStoryPackID)?.description ?? ""}
          </p>
        {/if}
      </div>
      <div class="mb-3 flex items-center justify-between">
        <h2 class="text-sm font-semibold uppercase text-slate-600">Stories</h2>
        <span class="text-xs text-slate-500">{stories?.length ?? 0}</span>
      </div>
      <div class="space-y-2">
        {#each stories as story}
          <button
            class:selected={story.story_id === selectedStoryId}
            class="story-button"
            on:click={() => withBusy(() => loadStory(story.story_id))}
          >
            <span class="block font-medium"
              >{story.label || story.story_id}</span
            >
            <span class="block text-xs text-slate-500"
              >turn {story.latest_turn} - {story.story_id}</span
            >
            <span class="block text-xs text-slate-500"
              >{story.ground_lore_pack_id || "legacy"} - {story.ground_lore_collection}</span
            >
            <span class="block text-xs text-slate-400">{story.updated_at}</span>
          </button>
        {/each}
        {#if stories?.length === 0 || !stories}
          <div
            class="rounded border border-dashed border-slate-300 p-4 text-sm text-slate-500"
          >
            No story workspaces under {status?.story_root ?? "data/stories"}.
          </div>
        {/if}
      </div>
    </aside>

    <section class="min-w-0 p-4">
      {#if error}
        <div
          class="mb-3 rounded border border-red-300 bg-red-50 p-3 text-sm text-red-800"
        >
          {error}
        </div>
      {/if}
      {#if notice}
        <div
          class="mb-3 rounded border border-emerald-300 bg-emerald-50 p-3 text-sm text-emerald-800"
        >
          {notice}
        </div>
      {/if}

      {#if detail}
        <div class="mb-4 flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 class="text-2xl font-semibold">{detail.summary.label}</h2>
            <p class="text-sm text-slate-500">
              {detail.summary.story_id} - turn {detail.summary.latest_turn} - {detail
                .summary.path}
            </p>
          </div>
          <div class="flex flex-wrap gap-2">
            {#each tabs as tab}
              <button
                class:tab-active={activeTab === tab}
                class="tab"
                on:click={() => (activeTab = tab)}
              >
                {tab}
              </button>
            {/each}
          </div>
        </div>

        {#if activeTab === "overview"}
          <div class="grid grid-cols-2 gap-4">
            <Panel title="State">
              <pre class="json">{pretty(detail.state)}</pre>
            </Panel>
            <Panel title="Bible">
              <pre class="json">{pretty(detail.bible)}</pre>
            </Panel>
            <Panel title="Current Packet">
              <pre class="json">{pretty(detail.packet)}</pre>
            </Panel>
            <Panel title={`Turns (${detail.turn_count})`}>
              <pre class="json">{pretty(detail.turns ?? [])}</pre>
            </Panel>
            <Panel title="Paths">
              <pre class="json">{pretty(detail.paths)}</pre>
            </Panel>
          </div>
        {:else if activeTab === "logs"}
          <Panel title="Logs">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <select
                bind:value={logCategory}
                class="input"
                on:change={() => withBusy(loadLogs)}
              >
                <option value="">all categories</option>
                {#each categories as category}
                  <option value={category}>{category}</option>
                {/each}
              </select>
              <select
                bind:value={logLevel}
                class="input"
                on:change={() => withBusy(loadLogs)}
              >
                <option value="">all levels</option>
                <option value="debug">debug</option>
                <option value="info">info</option>
                <option value="warn">warn</option>
                <option value="error">error</option>
              </select>
              <input
                class="input w-24"
                type="number"
                min="1"
                max="2000"
                bind:value={logTail}
              />
              <label class="flex items-center gap-1 text-xs text-slate-600">
                <input type="checkbox" bind:checked={logPolling} />
                poll
              </label>
              <button class="btn" on:click={() => withBusy(loadLogs)}
                >Apply</button
              >
            </div>
            <div class="grid grid-cols-[minmax(0,1fr)_420px] gap-3">
              <div class="overflow-auto">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th>time</th>
                      <th>level</th>
                      <th>category</th>
                      <th>event</th>
                      <th>message</th>
                      <th>fields</th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each logs as entry}
                      <tr
                        class:selected-row={selectedLog && logKey(entry) === logKey(selectedLog)}
                        class="cursor-pointer"
                        on:click={() => (selectedLog = entry)}
                      >
                        <td class="font-mono">{entry.time}</td>
                        <td>{entry.level}</td>
                        <td>{entry.category}</td>
                        <td>{entry.event}</td>
                        <td>{entry.message}</td>
                        <td class="max-w-[420px] font-mono">
                          {short(pretty(entry.fields ?? {}), 160)}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
              <div class="min-w-0">
                <div class="mb-2 flex items-center justify-between">
                  <h4 class="text-xs font-semibold uppercase tracking-wide text-slate-600">Selected Event</h4>
                  {#if selectedLog}
                    <button class="btn" on:click={() => (selectedLog = null)}>Clear</button>
                  {/if}
                </div>
                {#if selectedLog}
                  <div class="mb-3">
                    <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Fields</div>
                    <pre class="json max-h-[38vh]">{pretty(selectedLog.fields ?? {})}</pre>
                  </div>
                  <div>
                    <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Full Event</div>
                    <pre class="json max-h-[38vh]">{pretty(selectedLog)}</pre>
                  </div>
                {:else}
                  <div class="rounded border border-dashed border-slate-300 p-4 text-sm text-slate-500">
                    Select a log row to inspect complete fields and event JSON.
                  </div>
                {/if}
              </div>
            </div>
          </Panel>
        {:else if activeTab === "model-runs"}
          <Panel title="Model Runs">
            <FileTable
              files={modelRuns}
              onOpen={(path) => withBusy(() => loadFile(path))}
            />
          </Panel>
        {:else if activeTab === "index"}
          <div class="grid grid-cols-[360px_1fr] gap-4">
            <Panel title="Manifest">
              <pre class="json">{pretty(indexData?.manifest ?? {})}</pre>
            </Panel>
            <Panel title={`Documents (${indexData?.documents?.length ?? 0})`}>
              <pre class="json">{pretty(indexData?.documents ?? [])}</pre>
            </Panel>
          </div>
        {:else if activeTab === "files"}
          <div class="grid grid-cols-[260px_1fr] gap-4">
            <Panel title="Common Files">
              <div class="space-y-2">
                {#each commonFiles as path}
                  <button
                    class:selected={filePath === path}
                    class="story-button"
                    on:click={() => withBusy(() => loadFile(path))}
                  >
                    {path}
                  </button>
                {/each}
              </div>
            </Panel>
            <Panel title={filePath}>
              <pre class="json">{fileText}</pre>
            </Panel>
          </div>
        {:else if activeTab === "external"}
          <div class="grid grid-cols-2 gap-4">
            <Panel title="Bridge">
              <pre class="json">{pretty(status)}</pre>
            </Panel>
            <Panel title="Collections">
              <pre class="json">{pretty({
                  ground_lore_pack_id: detail.state.ground_lore_pack_id ?? detail.summary.ground_lore_pack_id,
                  ground_lore_collection: detail.state.ground_lore_collection ?? detail.summary.ground_lore_collection,
                  compatibility_global_lore_collection: detail.state.global_lore_collection,
                  story_collection: detail.state.story_collection,
                  bridge_url: status?.bridge_url,
                })}</pre>
            </Panel>
            <Panel title="Ground Lore Pack">
              <pre class="json">{pretty(detail.ground_lore_pack ?? {})}</pre>
            </Panel>
          </div>
        {:else if activeTab === "control"}
          <Panel title="Control Plane">
            <div class="grid grid-cols-2 gap-3">
              <ControlButton
                title="Rebuild Local Index"
                description="Recreate index/documents.jsonl and manifest.json from local bible, packet, and turns."
                action={() => control("rebuild-index")}
              />
              <ControlButton
                title="Reingest Story Index"
                description="Upsert current story index into Qdrant through the bridge."
                action={() => control("reingest-index")}
              />
              <ControlButton
                title="Refresh Writer Packet"
                description="Run the live writer refresh for the current story state."
                action={() => control("refresh-writer")}
              />
              <ControlButton
                title="Recommit Latest Turn"
                description="Commit the stored latest turn again if its canon payload validates."
                action={() =>
                  control("recommit-turn", {
                    turn: detail?.summary.latest_turn ?? 0,
                  })}
              />
              <ControlButton
                variant="danger"
                title="Delete Story"
                description="Permanently delete the local story directory, story Qdrant collection, and Honcho workspace after typed confirmation."
                action={() => deleteSelectedStory()}
              />
            </div>
          </Panel>
        {/if}
      {:else}
        <div
          class="rounded border border-slate-300 bg-white p-8 text-slate-500"
        >
          Select a story to inspect.
        </div>
      {/if}
    </section>
  </main>
</div>
