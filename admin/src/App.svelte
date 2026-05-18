<script lang="ts">
  import { onDestroy, onMount } from "svelte";
  import ControlButton from "./lib/ControlButton.svelte";
  import FileTable from "./lib/FileTable.svelte";
  import Panel from "./lib/Panel.svelte";
  import {
    backstageStreamURL,
    createStory,
    deleteGlobalLog,
    deleteGlobalLogs,
    deleteLog,
    deleteLogs,
    getFile,
    getBackstage,
    getGlobalBackstage,
    getGlobalLogDetail,
    getGlobalLogs,
    getIndex,
    getLogDetail,
    getLogs,
    getModelRuns,
    getStatus,
    getStory,
    globalBackstageStreamURL,
    listLorePacks,
    listStories,
    runControl,
    type AdminStatus,
    type BackstageMessage,
    type FileInfo,
    type IndexResponse,
    type LogEvent,
    type LorePack,
    type StoryDetail,
    type StorySummary,
  } from "./lib/api";

  const tabs = [
    "overview",
    "book",
    "behind-scenes",
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
    "narration/pending-turn.md",
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
  let globalLogs: LogEvent[] = [];
  let backstageMessages: BackstageMessage[] = [];
  let globalBackstageMessages: BackstageMessage[] = [];
  let selectedLog: LogEvent | null = null;
  let selectedGlobalLog: LogEvent | null = null;
  let selectedBackstageMessage: BackstageMessage | null = null;
  let selectedGlobalBackstageMessage: BackstageMessage | null = null;
  let selectedBackstageDetail: unknown = null;
  let selectedGlobalBackstageDetail: unknown = null;
  let selectedLogDetail: unknown = null;
  let selectedGlobalLogDetail: unknown = null;
  let selectedLogIDs = new Set<string>();
  let selectedGlobalLogIDs = new Set<string>();
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
  let backstageTail = 200;
  let backstageLive = true;
  let backstageRole = "";
  let backstageAdminDetail = false;
  let sidebarVisible = true;
  let busy = false;
  let error = "";
  let notice = "";
  let pollTimer: number | undefined;
  let backstageSource: EventSource | null = null;
  let backstageStreamKey = "";
  let storyChatEl: HTMLDivElement | null = null;
  let globalChatEl: HTMLDivElement | null = null;
  let storyBackstageCount = 0;
  let globalBackstageCount = 0;

  $: configureBackstageStream(activeTab, selectedStoryId, backstageLive, backstageTail);
  $: autoscrollBackstage();

  onMount(() => {
    void refreshAll();
    pollTimer = window.setInterval(() => {
      if (logPolling && activeTab === "logs" && selectedStoryId) {
        void loadLogsQuietly();
      }
      if (logPolling && activeTab === "global-logs") {
        void loadGlobalLogsQuietly();
      }
      if (!backstageLive && activeTab === "behind-scenes" && selectedStoryId) {
        void loadBackstageQuietly();
      }
      if (!backstageLive && activeTab === "global-backstage") {
        void loadGlobalBackstageQuietly();
      }
    }, 3000);
  });

  onDestroy(() => {
    if (pollTimer !== undefined) {
      window.clearInterval(pollTimer);
    }
    closeBackstageStream();
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
      await loadGlobalLogs();
      await loadGlobalBackstage();
    });
  }

  async function loadStory(storyId: string) {
    selectedStoryId = storyId;
    if (activeTab === "global-logs" || activeTab === "global-backstage") {
      activeTab = "overview";
    }
    detail = await getStory(storyId);
    await Promise.all([
      loadLogs(),
      loadBackstage(),
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
    selectedLogIDs = pruneSelectedLogIDs(selectedLogIDs, logs);
    categories = data.categories;
    const currentLog = selectedLog;
    if (currentLog && !logs.some((entry) => logKey(entry) === logKey(currentLog))) {
      selectedLog = null;
      selectedLogDetail = null;
    }
  }

  async function loadGlobalLogs() {
    const data = await getGlobalLogs({
      category: logCategory,
      level: logLevel,
      tail: logTail,
    });
    globalLogs = data.events;
    selectedGlobalLogIDs = pruneSelectedLogIDs(selectedGlobalLogIDs, globalLogs);
    categories = data.categories;
    const currentLog = selectedGlobalLog;
    if (currentLog && !globalLogs.some((entry) => logKey(entry) === logKey(currentLog))) {
      selectedGlobalLog = null;
      selectedGlobalLogDetail = null;
    }
  }

  async function loadBackstage() {
    if (!selectedStoryId) return;
    backstageMessages = await getBackstage(selectedStoryId, { tail: backstageTail });
    const current = selectedBackstageMessage;
    if (current && !backstageMessages.some((entry) => entry.id === current.id)) {
      selectedBackstageMessage = null;
      selectedBackstageDetail = null;
    }
  }

  async function loadGlobalBackstage() {
    globalBackstageMessages = await getGlobalBackstage({ tail: backstageTail });
    const current = selectedGlobalBackstageMessage;
    if (current && !globalBackstageMessages.some((entry) => entry.id === current.id)) {
      selectedGlobalBackstageMessage = null;
      selectedGlobalBackstageDetail = null;
    }
  }

  async function loadLogsQuietly() {
    try {
      await loadLogs();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function loadGlobalLogsQuietly() {
    try {
      await loadGlobalLogs();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function loadBackstageQuietly() {
    try {
      await loadBackstage();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function loadGlobalBackstageQuietly() {
    try {
      await loadGlobalBackstage();
    } catch (err) {
      error = err instanceof Error ? err.message : String(err);
    }
  }

  async function selectBackstageMessage(message: BackstageMessage) {
    selectedBackstageMessage = message;
    selectedBackstageDetail = null;
    if (selectedStoryId && message.detail_path) {
      selectedBackstageDetail = await getLogDetail(selectedStoryId, message.detail_path);
    }
  }

  async function selectGlobalBackstageMessage(message: BackstageMessage) {
    selectedGlobalBackstageMessage = message;
    selectedGlobalBackstageDetail = null;
    if (message.detail_path) {
      selectedGlobalBackstageDetail = await getGlobalLogDetail(message.detail_path);
    }
  }

  async function selectLog(entry: LogEvent) {
    selectedLog = entry;
    selectedLogDetail = null;
    if (selectedStoryId && entry.detail_path) {
      selectedLogDetail = await getLogDetail(selectedStoryId, entry.detail_path);
    }
  }

  async function selectGlobalLog(entry: LogEvent) {
    selectedGlobalLog = entry;
    selectedGlobalLogDetail = null;
    if (entry.detail_path) {
      selectedGlobalLogDetail = await getGlobalLogDetail(entry.detail_path);
    }
  }

  async function deleteStoryLogRow(entry: LogEvent) {
    if (!selectedStoryId || !entry.event_id) return;
    if (!confirm(`Delete log event ${entry.event}?`)) return;
    await withBusy(async () => {
      await deleteLog(selectedStoryId, entry.event_id);
      if (selectedLog && logKey(selectedLog) === logKey(entry)) {
        selectedLog = null;
        selectedLogDetail = null;
      }
      await loadLogs();
      notice = `deleted log event: ${entry.event_id}`;
    });
  }

  async function deleteGlobalLogRow(entry: LogEvent) {
    if (!entry.event_id) return;
    if (!confirm(`Delete global log event ${entry.event}?`)) return;
    await withBusy(async () => {
      await deleteGlobalLog(entry.event_id);
      if (selectedGlobalLog && logKey(selectedGlobalLog) === logKey(entry)) {
        selectedGlobalLog = null;
        selectedGlobalLogDetail = null;
      }
      await loadGlobalLogs();
      notice = `deleted global log event: ${entry.event_id}`;
    });
  }

  async function deleteSelectedStoryLogs() {
    if (!selectedStoryId || selectedLogIDs.size === 0) return;
    const eventIDs = [...selectedLogIDs];
    if (!confirm(`Delete ${eventIDs.length} story log events?`)) return;
    await withBusy(async () => {
      await deleteLogs(selectedStoryId, eventIDs);
      if (selectedLog?.event_id && selectedLogIDs.has(selectedLog.event_id)) {
        selectedLog = null;
        selectedLogDetail = null;
      }
      selectedLogIDs = new Set<string>();
      await loadLogs();
      notice = `deleted ${eventIDs.length} story log events`;
    });
  }

  async function deleteSelectedGlobalLogs() {
    if (selectedGlobalLogIDs.size === 0) return;
    const eventIDs = [...selectedGlobalLogIDs];
    if (!confirm(`Delete ${eventIDs.length} global log events?`)) return;
    await withBusy(async () => {
      await deleteGlobalLogs(eventIDs);
      if (selectedGlobalLog?.event_id && selectedGlobalLogIDs.has(selectedGlobalLog.event_id)) {
        selectedGlobalLog = null;
        selectedGlobalLogDetail = null;
      }
      selectedGlobalLogIDs = new Set<string>();
      await loadGlobalLogs();
      notice = `deleted ${eventIDs.length} global log events`;
    });
  }

  async function copyStoryLogMarkdown(entry: LogEvent) {
    if (!selectedStoryId) return;
    await withBusy(async () => {
      const detail = entry.detail_path
        ? await getLogDetail(selectedStoryId, entry.detail_path)
        : null;
      await copyText(formatLogMarkdown(entry, detail, "story"));
      notice = `copied log event: ${entry.event_id || entry.event}`;
    });
  }

  async function copyGlobalLogMarkdown(entry: LogEvent) {
    await withBusy(async () => {
      const detail = entry.detail_path
        ? await getGlobalLogDetail(entry.detail_path)
        : null;
      await copyText(formatLogMarkdown(entry, detail, "global"));
      notice = `copied global log event: ${entry.event_id || entry.event}`;
    });
  }

  async function copyBackstageMarkdown(message: BackstageMessage, scope: "story" | "global") {
    const detailPayload = message.detail_path
      ? scope === "story" && selectedStoryId
        ? await getLogDetail(selectedStoryId, message.detail_path)
        : await getGlobalLogDetail(message.detail_path)
      : null;
    await copyText(formatBackstageMarkdown(message, detailPayload, scope));
    notice = `copied backstage message: ${message.id}`;
  }

  async function openBackstageArtifact(path: string) {
    if (!selectedStoryId || !path) return;
    await withBusy(async () => {
      await loadFile(path);
      activeTab = "files";
    });
  }

  function configureBackstageStream(tab: string, storyId: string, live: boolean, tail: number) {
    const wantsStory = live && tab === "behind-scenes" && !!storyId;
    const wantsGlobal = live && tab === "global-backstage";
    if (!wantsStory && !wantsGlobal) {
      closeBackstageStream();
      return;
    }
    const key = wantsStory ? `story:${storyId}:${tail}` : `global:${tail}`;
    if (key === backstageStreamKey && backstageSource) {
      return;
    }
    closeBackstageStream();
    backstageStreamKey = key;
    const url = wantsStory ? backstageStreamURL(storyId, tail) : globalBackstageStreamURL(tail);
    const source = new EventSource(url);
    source.addEventListener("backstage", (event) => {
      try {
        const payload = JSON.parse((event as MessageEvent).data) as { messages?: BackstageMessage[] };
        if (payload.messages) {
          if (wantsStory) backstageMessages = payload.messages;
          else globalBackstageMessages = payload.messages;
        }
      } catch (err) {
        error = err instanceof Error ? err.message : String(err);
      }
    });
    source.onerror = () => {
      error = "backstage stream disconnected";
      closeBackstageStream();
    };
    backstageSource = source;
  }

  function closeBackstageStream() {
    if (backstageSource) {
      backstageSource.close();
      backstageSource = null;
    }
    backstageStreamKey = "";
  }

  function autoscrollBackstage() {
    if (!backstageLive) return;
    if (activeTab === "behind-scenes" && backstageMessages.length !== storyBackstageCount) {
      storyBackstageCount = backstageMessages.length;
      queueMicrotask(() => storyChatEl?.scrollTo({ top: storyChatEl.scrollHeight }));
    }
    if (activeTab === "global-backstage" && globalBackstageMessages.length !== globalBackstageCount) {
      globalBackstageCount = globalBackstageMessages.length;
      queueMicrotask(() => globalChatEl?.scrollTo({ top: globalChatEl.scrollHeight }));
    }
  }

  function onBackstageKey(event: KeyboardEvent, open: () => void) {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    open();
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
      selectedLogIDs = new Set<string>();
      selectedLog = null;
      selectedLogDetail = null;
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

  function bookText(value: string) {
    return value
      .replace(/^# [^\n]+\n\n/, "")
      .replace(/^<!--[\s\S]*?-->\n\n/, "")
      .trim();
  }

  function logKey(entry: LogEvent) {
    if (entry.event_id) return entry.event_id;
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

  function pruneSelectedLogIDs(selected: Set<string>, events: LogEvent[]) {
    const visible = new Set(logEventIDs(events));
    return new Set([...selected].filter((eventID) => visible.has(eventID)));
  }

  function toggleStoryLogSelection(entry: LogEvent) {
    if (!entry.event_id) return;
    const next = new Set(selectedLogIDs);
    if (next.has(entry.event_id)) next.delete(entry.event_id);
    else next.add(entry.event_id);
    selectedLogIDs = next;
  }

  function toggleGlobalLogSelection(entry: LogEvent) {
    if (!entry.event_id) return;
    const next = new Set(selectedGlobalLogIDs);
    if (next.has(entry.event_id)) next.delete(entry.event_id);
    else next.add(entry.event_id);
    selectedGlobalLogIDs = next;
  }

  function selectAllStoryLogs() {
    selectedLogIDs = new Set(logEventIDs(logs));
  }

  function selectAllGlobalLogs() {
    selectedGlobalLogIDs = new Set(logEventIDs(globalLogs));
  }

  function logEventIDs(events: LogEvent[]) {
    const out: string[] = [];
    for (const entry of events) {
      if (entry.event_id) out.push(entry.event_id);
    }
    return out;
  }

  async function copyText(text: string) {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "true");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    document.execCommand("copy");
    textarea.remove();
  }

  function formatLogMarkdown(entry: LogEvent, detailPayload: unknown, scope: "story" | "global") {
    const lines = [
      `# ${scope === "global" ? "Global" : "Story"} Log: ${entry.event || "(unnamed event)"}`,
      "",
      "| Field | Value |",
      "| --- | --- |",
      `| Time | ${markdownCell(entry.time)} |`,
      `| Level | ${markdownCell(entry.level)} |`,
      `| Category | ${markdownCell(entry.category)} |`,
      `| Event | ${markdownCell(entry.event)} |`,
      `| Event ID | ${markdownCell(entry.event_id)} |`,
    ];
    if (entry.story_id) lines.push(`| Story ID | ${markdownCell(entry.story_id)} |`);
    if (entry.turn !== undefined) lines.push(`| Turn | ${entry.turn} |`);
    if (entry.packet_id) lines.push(`| Packet ID | ${markdownCell(entry.packet_id)} |`);
    if (entry.node_id) lines.push(`| Node ID | ${markdownCell(entry.node_id)} |`);
    if (entry.run_id) lines.push(`| Run ID | ${markdownCell(entry.run_id)} |`);
    if (entry.message) lines.push(`| Message | ${markdownCell(entry.message)} |`);
    if (entry.detail_path) lines.push(`| Detail Path | ${markdownCell(entry.detail_path)} |`);

    lines.push(
      "",
      "## Fields",
      "````json",
      pretty(entry.fields ?? {}),
      "````",
      "",
      "## Detail",
      "````json",
      detailPayload === null ? "null" : pretty(detailPayload),
      "````",
      "",
      "## Event JSON",
      "````json",
      pretty(entry),
      "````",
      "",
    );
    return lines.join("\n");
  }

  function markdownCell(value: unknown) {
    return String(value ?? "")
      .replace(/\|/g, "\\|")
      .replace(/\r?\n/g, "<br>");
  }

  function filteredBackstage(messages: BackstageMessage[]) {
    if (!backstageRole) return messages;
    return messages.filter((message) => message.role === backstageRole);
  }

  function backstageRoles(messages: BackstageMessage[]) {
    return [...new Set(messages.map((message) => message.role).filter(Boolean))].sort();
  }

  function hasModelMeta(message: BackstageMessage) {
    const model = message.model;
    return Boolean(
      model?.provider_type ||
        model?.model_provider ||
        model?.model ||
        model?.reasoning_level ||
        model?.service_tier,
    );
  }

  function modelChip(message: BackstageMessage) {
    const model = message.model;
    if (!model) return "";
    return [model.model_provider, model.model].filter(Boolean).join(" / ") || model.provider_type || "model";
  }

  function modelTooltip(message: BackstageMessage) {
    const model = message.model;
    if (!model) return "";
    return [
      ["Provider type", model.provider_type],
      ["Model provider", model.model_provider],
      ["Model", model.model],
      ["Reasoning", model.reasoning_level],
      ["Service tier", model.service_tier],
    ]
      .filter(([, value]) => Boolean(value))
      .map(([label, value]) => `${label}: ${value}`)
      .join("\n");
  }

  function isOpenableArtifact(path: string) {
    return /\.(json|jsonl|md|txt|log)$/i.test(path);
  }

  function formatBackstageMarkdown(message: BackstageMessage, detailPayload: unknown, scope: "story" | "global") {
    const lines = [
      `# ${scope === "global" ? "Global" : "Story"} Backstage: ${message.speaker}`,
      "",
      "| Field | Value |",
      "| --- | --- |",
      `| Time | ${markdownCell(message.time)} |`,
      `| Speaker | ${markdownCell(message.speaker)} |`,
      `| Role | ${markdownCell(message.role)} |`,
      `| Kind | ${markdownCell(message.kind)} |`,
      `| Status | ${markdownCell(message.status)} |`,
      `| Visibility | ${markdownCell(message.visibility)} |`,
    ];
    if (message.story_id) lines.push(`| Story ID | ${markdownCell(message.story_id)} |`);
    if (message.turn !== undefined) lines.push(`| Turn | ${message.turn} |`);
    if (message.packet_id) lines.push(`| Packet ID | ${markdownCell(message.packet_id)} |`);
    if (message.node_id) lines.push(`| Node ID | ${markdownCell(message.node_id)} |`);
    if (message.run_id) lines.push(`| Run ID | ${markdownCell(message.run_id)} |`);
    if (message.model?.provider_type) lines.push(`| Provider Type | ${markdownCell(message.model.provider_type)} |`);
    if (message.model?.model_provider) lines.push(`| Model Provider | ${markdownCell(message.model.model_provider)} |`);
    if (message.model?.model) lines.push(`| Model | ${markdownCell(message.model.model)} |`);
    if (message.model?.reasoning_level) lines.push(`| Reasoning | ${markdownCell(message.model.reasoning_level)} |`);
    if (message.model?.service_tier) lines.push(`| Service Tier | ${markdownCell(message.model.service_tier)} |`);
    if (message.source_event) lines.push(`| Source Event | ${markdownCell(message.source_event)} |`);
    if (message.source_event_id) lines.push(`| Source Event ID | ${markdownCell(message.source_event_id)} |`);
    if (message.detail_path) lines.push(`| Detail Path | ${markdownCell(message.detail_path)} |`);
    lines.push(
      "",
      "## Message",
      "",
      message.text || message.preview || "",
      "",
      "## Artifacts",
      "````json",
      pretty(message.artifact_paths ?? []),
      "````",
      "",
      "## Detail",
      "````json",
      detailPayload === null ? "null" : pretty(detailPayload),
      "````",
      "",
      "## Message JSON",
      "````json",
      pretty(message),
      "````",
      "",
    );
    return lines.join("\n");
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
        <button
          class="btn"
          aria-pressed={sidebarVisible}
          on:click={() => (sidebarVisible = !sidebarVisible)}
        >
          {sidebarVisible ? "Hide sidebar" : "Show sidebar"}
        </button>
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

  <main class={sidebarVisible ? "grid min-h-[calc(100vh-57px)] grid-cols-[320px_1fr]" : "grid min-h-[calc(100vh-57px)] grid-cols-1"}>
    {#if sidebarVisible}
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
      <div class="mb-4">
        <button
          class:selected={activeTab === "global-backstage"}
          class="story-button mb-2"
          on:click={() => withBusy(async () => { activeTab = "global-backstage"; await loadGlobalBackstage(); })}
        >
          <span class="block font-medium">Behind The Scenes</span>
          <span class="block text-xs text-slate-500">global model calls as office chat</span>
        </button>
        <button
          class:selected={activeTab === "global-logs"}
          class="story-button"
          on:click={() => withBusy(async () => { activeTab = "global-logs"; await loadGlobalLogs(); })}
        >
          <span class="block font-medium">Global Logs</span>
          <span class="block text-xs text-slate-500">startup, seed, bridge, and system events</span>
        </button>
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
    {/if}

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

      {#if activeTab === "global-backstage"}
        <Panel title="Global Behind The Scenes">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <select bind:value={backstageRole} class="input">
              <option value="">all roles</option>
              {#each backstageRoles(globalBackstageMessages) as role}
                <option value={role}>{role}</option>
              {/each}
            </select>
            <input
              class="input w-24"
              type="number"
              min="1"
              max="2000"
              bind:value={backstageTail}
            />
            <label class="flex items-center gap-1 text-xs text-slate-600">
              <input type="checkbox" bind:checked={backstageLive} />
              live
            </label>
            <label class="flex items-center gap-1 text-xs text-slate-600">
              <input type="checkbox" bind:checked={backstageAdminDetail} />
              detail
            </label>
            <button class="btn" on:click={() => withBusy(loadGlobalBackstage)}>Apply</button>
          </div>
          <div class={selectedGlobalBackstageMessage ? "grid grid-cols-[minmax(0,1fr)_420px] gap-3" : "grid gap-3"}>
            <div class="chat-room" bind:this={globalChatEl}>
              {#each filteredBackstage(globalBackstageMessages) as message}
                <div
                  class:failed={message.status === "failed"}
                  class:pending={message.status === "pending"}
                  class="chat-bubble"
                  role="button"
                  tabindex="0"
                  on:click={() => withBusy(() => selectGlobalBackstageMessage(message))}
                  on:keydown={(event) => onBackstageKey(event, () => void withBusy(() => selectGlobalBackstageMessage(message)))}
                >
                  <div class="mb-2 flex flex-wrap items-center gap-2">
                    <span class="role-chip">@{message.speaker}</span>
                    <span class="chip">{message.kind}</span>
                    <span class="chip">{message.status}</span>
                    {#if hasModelMeta(message)}
                      <span class="chip" title={modelTooltip(message)}>{modelChip(message)}</span>
                    {/if}
                    {#if message.turn}<span class="chip">turn {message.turn}</span>{/if}
                    {#if message.run_id}<span class="chip">{message.run_id}</span>{/if}
                    <span class="ml-auto font-mono text-[11px] text-slate-500">{message.time}</span>
                  </div>
                  <div class="whitespace-pre-wrap text-sm leading-relaxed">{message.preview || message.text}</div>
                  <div class="mt-2 flex flex-wrap gap-1">
                    {#if message.detail_path}
                      <button class="btn-mini" on:click|stopPropagation={() => withBusy(() => selectGlobalBackstageMessage(message))}>Detail</button>
                    {/if}
                    <button class="btn-mini" on:click|stopPropagation={() => copyBackstageMarkdown(message, "global")}>Copy</button>
                  </div>
                </div>
              {/each}
              {#if filteredBackstage(globalBackstageMessages).length === 0}
                <div class="rounded border border-dashed border-slate-300 p-4 text-sm text-slate-500">
                  No backstage messages yet.
                </div>
              {/if}
            </div>
            {#if selectedGlobalBackstageMessage}
            <div class="min-w-0">
              <div class="mb-2 flex items-center justify-between">
                <h4 class="text-xs font-semibold uppercase tracking-wide text-slate-600">Selected Message</h4>
                <button class="btn" on:click={() => { selectedGlobalBackstageMessage = null; selectedGlobalBackstageDetail = null; }}>Clear</button>
              </div>
              <pre class="json max-h-[38vh]">{selectedGlobalBackstageMessage.text}</pre>
              {#if backstageAdminDetail}
                <div class="mt-3">
                  <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Detail</div>
                  <pre class="json max-h-[38vh]">{pretty(selectedGlobalBackstageDetail ?? {})}</pre>
                </div>
              {/if}
            </div>
            {/if}
          </div>
        </Panel>
      {:else if activeTab === "global-logs"}
        <Panel title="Global Logs">
          <div class="mb-3 flex flex-wrap items-center gap-2">
            <select
              bind:value={logCategory}
              class="input"
              on:change={() => withBusy(loadGlobalLogs)}
            >
              <option value="">all categories</option>
              {#each categories as category}
                <option value={category}>{category}</option>
              {/each}
            </select>
            <select
              bind:value={logLevel}
              class="input"
              on:change={() => withBusy(loadGlobalLogs)}
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
            <button class="btn" on:click={() => withBusy(loadGlobalLogs)}
              >Apply</button
            >
            <button class="btn" disabled={globalLogs.length === 0} on:click={selectAllGlobalLogs}>
              Select all rows
            </button>
            <button class="btn" disabled={selectedGlobalLogIDs.size === 0} on:click={() => (selectedGlobalLogIDs = new Set<string>())}>
              Clear selection
            </button>
            <button
              class="btn-danger"
              disabled={selectedGlobalLogIDs.size === 0}
              on:click={deleteSelectedGlobalLogs}
            >
              Delete selected ({selectedGlobalLogIDs.size})
            </button>
          </div>
          <div class={selectedGlobalLog ? "grid grid-cols-[minmax(0,1fr)_420px] gap-3" : "grid gap-3"}>
            <div class="overflow-auto">
              <table class="data-table">
                <thead>
                  <tr>
                    <th class="w-8"></th>
                    <th>time</th>
                    <th>level</th>
                    <th>category</th>
                    <th>event</th>
                    <th>story</th>
                    <th>message</th>
                    <th>fields</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {#each globalLogs as entry}
                    <tr
                      class:selected-row={selectedGlobalLog && logKey(entry) === logKey(selectedGlobalLog)}
                      class="cursor-pointer"
                      on:click={() => withBusy(() => selectGlobalLog(entry))}
                    >
                      <td>
                        <input
                          type="checkbox"
                          checked={entry.event_id ? selectedGlobalLogIDs.has(entry.event_id) : false}
                          disabled={!entry.event_id}
                          on:click|stopPropagation={() => toggleGlobalLogSelection(entry)}
                        />
                      </td>
                      <td class="font-mono">{entry.time}</td>
                      <td>{entry.level}</td>
                      <td>{entry.category}</td>
                      <td>{entry.event}</td>
                      <td>{entry.story_id ?? ""}</td>
                      <td>{entry.message}</td>
                      <td class="max-w-[420px] font-mono">
                        {short(pretty(entry.fields ?? {}), 160)}
                      </td>
                      <td class="text-right">
                        <div class="flex justify-end gap-1">
                          <button
                            class="btn-mini"
                            on:click|stopPropagation={() => copyGlobalLogMarkdown(entry)}
                          >
                            Copy
                          </button>
                          <button
                            class="btn-danger"
                            disabled={!entry.event_id}
                            on:click|stopPropagation={() => deleteGlobalLogRow(entry)}
                          >
                            Delete
                          </button>
                        </div>
                      </td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
            {#if selectedGlobalLog}
            <div class="min-w-0">
              <div class="mb-2 flex items-center justify-between">
                <h4 class="text-xs font-semibold uppercase tracking-wide text-slate-600">Selected Global Event</h4>
                <button class="btn" on:click={() => { selectedGlobalLog = null; selectedGlobalLogDetail = null; }}>Clear</button>
              </div>
              {#if selectedGlobalLogDetail}
                <div class="mb-3">
                  <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Detail</div>
                  <pre class="json max-h-[38vh]">{pretty(selectedGlobalLogDetail)}</pre>
                </div>
              {/if}
              <div class="mb-3">
                <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Fields</div>
                <pre class="json max-h-[38vh]">{pretty(selectedGlobalLog.fields ?? {})}</pre>
              </div>
              <div>
                <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Full Event</div>
                <pre class="json max-h-[38vh]">{pretty(selectedGlobalLog)}</pre>
              </div>
            </div>
            {/if}
          </div>
        </Panel>
      {:else if detail}
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
        {:else if activeTab === "book"}
          <Panel title={`Book (${detail.narration?.length ?? 0})`}>
            {#if detail.narration?.length}
              <div class="space-y-8">
                {#each detail.narration as chapter}
                  <article class="max-w-4xl border-b border-slate-200 pb-8 last:border-b-0">
                    <div class="mb-3 flex items-center justify-between gap-3">
                      <h3 class="text-lg font-semibold">
                        {chapter.pending ? "Pending turn" : `Turn ${chapter.turn ?? ""}`}
                      </h3>
                      <button class="btn" on:click={() => withBusy(() => loadFile(chapter.path))}>
                        Open source
                      </button>
                    </div>
                    <div class="whitespace-pre-wrap font-serif text-[17px] leading-8 text-slate-900">
                      {bookText(chapter.text)}
                    </div>
                  </article>
                {/each}
              </div>
            {:else}
              <div class="rounded border border-dashed border-slate-300 p-4 text-sm text-slate-500">
                No narration files saved yet.
              </div>
            {/if}
          </Panel>
        {:else if activeTab === "behind-scenes"}
          <Panel title="Behind The Scenes">
            <div class="mb-3 flex flex-wrap items-center gap-2">
              <select bind:value={backstageRole} class="input">
                <option value="">all roles</option>
                {#each backstageRoles(backstageMessages) as role}
                  <option value={role}>{role}</option>
                {/each}
              </select>
              <input
                class="input w-24"
                type="number"
                min="1"
                max="2000"
                bind:value={backstageTail}
              />
              <label class="flex items-center gap-1 text-xs text-slate-600">
                <input type="checkbox" bind:checked={backstageLive} />
                live
              </label>
              <label class="flex items-center gap-1 text-xs text-slate-600">
                <input type="checkbox" bind:checked={backstageAdminDetail} />
                detail
              </label>
              <button class="btn" on:click={() => withBusy(loadBackstage)}>Apply</button>
            </div>
            <div class={selectedBackstageMessage ? "grid grid-cols-[minmax(0,1fr)_420px] gap-3" : "grid gap-3"}>
              <div class="chat-room" bind:this={storyChatEl}>
                {#each filteredBackstage(backstageMessages) as message}
                  <div
                    class:failed={message.status === "failed"}
                    class:pending={message.status === "pending"}
                    class="chat-bubble"
                    role="button"
                    tabindex="0"
                    on:click={() => withBusy(() => selectBackstageMessage(message))}
                    on:keydown={(event) => onBackstageKey(event, () => void withBusy(() => selectBackstageMessage(message)))}
                  >
                    <div class="mb-2 flex flex-wrap items-center gap-2">
                      <span class="role-chip">@{message.speaker}</span>
                      <span class="chip">{message.kind}</span>
                      <span class="chip">{message.status}</span>
                      {#if hasModelMeta(message)}
                        <span class="chip" title={modelTooltip(message)}>{modelChip(message)}</span>
                      {/if}
                      {#if message.turn}<span class="chip">turn {message.turn}</span>{/if}
                      {#if message.node_id}<span class="chip">{message.node_id}</span>{/if}
                      {#if message.run_id}<span class="chip">{message.run_id}</span>{/if}
                      <span class="ml-auto font-mono text-[11px] text-slate-500">{message.time}</span>
                    </div>
                    <div class="whitespace-pre-wrap text-sm leading-relaxed">{message.preview || message.text}</div>
                    <div class="mt-2 flex flex-wrap gap-1">
                      {#each message.artifact_paths ?? [] as path}
                        {#if isOpenableArtifact(path)}
                          <button class="btn-mini" on:click|stopPropagation={() => openBackstageArtifact(path)}>
                            {short(path, 42)}
                          </button>
                        {:else}
                          <span class="chip">{short(path, 42)}</span>
                        {/if}
                      {/each}
                      {#if message.detail_path}
                        <button class="btn-mini" on:click|stopPropagation={() => withBusy(() => selectBackstageMessage(message))}>Detail</button>
                      {/if}
                      <button class="btn-mini" on:click|stopPropagation={() => copyBackstageMarkdown(message, "story")}>Copy</button>
                    </div>
                  </div>
                {/each}
                {#if filteredBackstage(backstageMessages).length === 0}
                  <div class="rounded border border-dashed border-slate-300 p-4 text-sm text-slate-500">
                    No backstage messages yet.
                  </div>
                {/if}
              </div>
              {#if selectedBackstageMessage}
              <div class="min-w-0">
                <div class="mb-2 flex items-center justify-between">
                  <h4 class="text-xs font-semibold uppercase tracking-wide text-slate-600">Selected Message</h4>
                  <button class="btn" on:click={() => { selectedBackstageMessage = null; selectedBackstageDetail = null; }}>Clear</button>
                </div>
                <pre class="json max-h-[38vh]">{selectedBackstageMessage.text}</pre>
                {#if selectedBackstageMessage.artifact_paths?.length}
                  <div class="mt-3">
                    <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Artifacts</div>
                    <div class="flex flex-wrap gap-1">
                      {#each selectedBackstageMessage.artifact_paths as path}
                        {#if isOpenableArtifact(path)}
                          <button class="btn-mini" on:click={() => openBackstageArtifact(path)}>
                            {path}
                          </button>
                        {:else}
                          <span class="chip">{path}</span>
                        {/if}
                      {/each}
                    </div>
                  </div>
                {/if}
                {#if backstageAdminDetail}
                  <div class="mt-3">
                    <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Detail</div>
                    <pre class="json max-h-[38vh]">{pretty(selectedBackstageDetail ?? {})}</pre>
                  </div>
                {/if}
              </div>
              {/if}
            </div>
          </Panel>
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
              <button class="btn" disabled={logs.length === 0} on:click={selectAllStoryLogs}>
                Select all rows
              </button>
              <button class="btn" disabled={selectedLogIDs.size === 0} on:click={() => (selectedLogIDs = new Set<string>())}>
                Clear selection
              </button>
              <button
                class="btn-danger"
                disabled={selectedLogIDs.size === 0}
                on:click={deleteSelectedStoryLogs}
              >
                Delete selected ({selectedLogIDs.size})
              </button>
            </div>
            <div class={selectedLog ? "grid grid-cols-[minmax(0,1fr)_420px] gap-3" : "grid gap-3"}>
              <div class="overflow-auto">
                <table class="data-table">
                  <thead>
                    <tr>
                      <th class="w-8"></th>
                      <th>time</th>
                      <th>level</th>
                      <th>category</th>
                      <th>event</th>
                      <th>message</th>
                      <th>fields</th>
                      <th></th>
                    </tr>
                  </thead>
                  <tbody>
                    {#each logs as entry}
                      <tr
                        class:selected-row={selectedLog && logKey(entry) === logKey(selectedLog)}
                        class="cursor-pointer"
                        on:click={() => withBusy(() => selectLog(entry))}
                      >
                        <td>
                          <input
                            type="checkbox"
                            checked={entry.event_id ? selectedLogIDs.has(entry.event_id) : false}
                            disabled={!entry.event_id}
                            on:click|stopPropagation={() => toggleStoryLogSelection(entry)}
                          />
                        </td>
                        <td class="font-mono">{entry.time}</td>
                        <td>{entry.level}</td>
                        <td>{entry.category}</td>
                        <td>{entry.event}</td>
                        <td>{entry.message}</td>
                        <td class="max-w-[420px] font-mono">
                          {short(pretty(entry.fields ?? {}), 160)}
                        </td>
                        <td class="text-right">
                          <div class="flex justify-end gap-1">
                            <button
                              class="btn-mini"
                              on:click|stopPropagation={() => copyStoryLogMarkdown(entry)}
                            >
                              Copy
                            </button>
                            <button
                              class="btn-danger"
                              disabled={!entry.event_id}
                              on:click|stopPropagation={() => deleteStoryLogRow(entry)}
                            >
                              Delete
                            </button>
                          </div>
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              </div>
              {#if selectedLog}
              <div class="min-w-0">
                <div class="mb-2 flex items-center justify-between">
                  <h4 class="text-xs font-semibold uppercase tracking-wide text-slate-600">Selected Event</h4>
                  <button class="btn" on:click={() => { selectedLog = null; selectedLogDetail = null; }}>Clear</button>
                </div>
                {#if selectedLogDetail}
                  <div class="mb-3">
                    <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Detail</div>
                    <pre class="json max-h-[38vh]">{pretty(selectedLogDetail)}</pre>
                  </div>
                {/if}
                <div class="mb-3">
                  <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Fields</div>
                  <pre class="json max-h-[38vh]">{pretty(selectedLog.fields ?? {})}</pre>
                </div>
                <div>
                  <div class="mb-1 text-xs font-semibold uppercase tracking-wide text-slate-500">Full Event</div>
                  <pre class="json max-h-[38vh]">{pretty(selectedLog)}</pre>
                </div>
              </div>
              {/if}
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
