export type StorySummary = {
  story_id: string;
  title: string;
  label: string;
  latest_turn: number;
  updated_at: string;
  created_at: string;
  path: string;
  ground_lore_pack_id: string;
  ground_lore_collection: string;
};

export type LorePack = {
  id: string;
  label: string;
  description: string;
  collection: string;
  embedding_model: string;
  artifact_roots: Record<string, string>;
  builder: string;
  status: string;
};

export type AdminStatus = {
  bridge_url: string;
  ok: boolean;
  error: string;
  story_root: string;
  health: {
    status?: string;
    honcho_live?: boolean;
    voyage_configured?: boolean;
    voyage_model?: string;
    qdrant_url?: string;
  };
};

export type StoryDetail = {
  summary: StorySummary;
  state: Record<string, unknown>;
  bible: Record<string, unknown>;
  packet: Record<string, unknown>;
  turns: Record<string, unknown>[];
  turn_count: number;
  narration: NarrationFile[];
  paths: Record<string, string>;
  ground_lore_pack?: LorePack;
};

export type NarrationFile = {
  path: string;
  name: string;
  turn?: number;
  pending?: boolean;
  text: string;
  size: number;
  mod_time: string;
};

export type LogEvent = {
  event_id: string;
  time: string;
  level: string;
  category: string;
  event: string;
  story_id?: string;
  turn?: number;
  packet_id?: string;
  node_id?: string;
  run_id?: string;
  message?: string;
  fields?: Record<string, unknown>;
  detail_path?: string;
};

export type BackstageMessage = {
  id: string;
  time: string;
  story_id?: string;
  turn?: number;
  packet_id?: string;
  node_id?: string;
  run_id?: string;
  role: string;
  speaker: string;
  kind: string;
  status: string;
  text: string;
  preview: string;
  preview_truncated?: boolean;
  tool_output_truncated?: boolean;
  source_event_id?: string;
  source_event?: string;
  source_category?: string;
  detail_path?: string;
  artifact_paths?: string[];
  visibility: string;
  model?: ModelMetadata;
};

export type ModelMetadata = {
  provider_type?: string;
  model_provider?: string;
  model?: string;
  reasoning_level?: string;
  service_tier?: string;
};

export type ActorSummary = {
  actor_id: string;
  display_name: string;
  card_source: string;
  card_preview: string;
};

export type ActorChatMessage = {
  role: string;
  text: string;
};

export type ActorChatResponse = {
  actor_id: string;
  interior?: string;
  dialogue?: string;
  action?: string;
  warnings?: string[];
  model?: ModelMetadata;
};

export type FileInfo = {
  path: string;
  name: string;
  size: number;
  mod_time: string;
  is_dir: boolean;
  category?: string;
};

export type IndexResponse = {
  collection: string;
  manifest: Record<string, unknown> | null;
  documents: Record<string, unknown>[];
};

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      'content-type': 'application/json',
      ...(init?.headers ?? {})
    }
  });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export async function getStatus(): Promise<AdminStatus> {
  return request<AdminStatus>('/admin/api/status');
}

export async function listStories(): Promise<StorySummary[]> {
  const data = await request<{ stories: StorySummary[] }>('/admin/api/stories');
  return data.stories;
}

export async function listLorePacks(): Promise<LorePack[]> {
  const data = await request<{ packs: LorePack[] }>('/admin/api/lore-packs');
  return data.packs;
}

export async function createStory(groundLorePackID: string): Promise<{ story_id: string; summary: StorySummary }> {
  return request<{ story_id: string; summary: StorySummary }>('/admin/api/stories', {
    method: 'POST',
    body: JSON.stringify({ ground_lore_pack_id: groundLorePackID })
  });
}

export async function getStory(storyId: string): Promise<StoryDetail> {
  return request<StoryDetail>(`/admin/api/stories/${encodeURIComponent(storyId)}`);
}

export async function getActors(storyId: string): Promise<ActorSummary[]> {
  const data = await request<{ actors: ActorSummary[] }>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/actors`
  );
  return data.actors;
}

export async function chatWithActor(
  storyId: string,
  body: { actor_id: string; message: string; history?: ActorChatMessage[] }
): Promise<ActorChatResponse> {
  return request<ActorChatResponse>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/actor-chat`,
    {
      method: 'POST',
      body: JSON.stringify(body)
    }
  );
}

export async function getLogs(
  storyId: string,
  filters: { category?: string; level?: string; tail?: number }
): Promise<{ events: LogEvent[]; categories: string[] }> {
  const params = new URLSearchParams();
  if (filters.category) params.set('category', filters.category);
  if (filters.level) params.set('level', filters.level);
  if (filters.tail) params.set('tail', String(filters.tail));
  return request<{ events: LogEvent[]; categories: string[] }>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/logs?${params.toString()}`
  );
}

export async function getGlobalLogs(
  filters: { category?: string; level?: string; tail?: number }
): Promise<{ events: LogEvent[]; categories: string[] }> {
  const params = new URLSearchParams();
  if (filters.category) params.set('category', filters.category);
  if (filters.level) params.set('level', filters.level);
  if (filters.tail) params.set('tail', String(filters.tail));
  return request<{ events: LogEvent[]; categories: string[] }>(
    `/admin/api/logs?${params.toString()}`
  );
}

export async function getBackstage(
  storyId: string,
  filters: { tail?: number } = {}
): Promise<BackstageMessage[]> {
  const params = new URLSearchParams();
  if (filters.tail) params.set('tail', String(filters.tail));
  const data = await request<{ messages: BackstageMessage[] }>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/backstage?${params.toString()}`
  );
  return data.messages;
}

export async function getGlobalBackstage(
  filters: { tail?: number } = {}
): Promise<BackstageMessage[]> {
  const params = new URLSearchParams();
  if (filters.tail) params.set('tail', String(filters.tail));
  const data = await request<{ messages: BackstageMessage[] }>(
    `/admin/api/backstage?${params.toString()}`
  );
  return data.messages;
}

export function backstageStreamURL(storyId: string, tail = 50): string {
  const params = new URLSearchParams({ tail: String(tail) });
  return `/admin/api/stories/${encodeURIComponent(storyId)}/backstage/stream?${params.toString()}`;
}

export function globalBackstageStreamURL(tail = 50): string {
  const params = new URLSearchParams({ tail: String(tail) });
  return `/admin/api/backstage/stream?${params.toString()}`;
}

export async function getLogDetail(storyId: string, path: string): Promise<unknown> {
  const params = new URLSearchParams({ path });
  const data = await request<{ detail: unknown }>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/logs/detail?${params.toString()}`
  );
  return data.detail;
}

export async function deleteLogs(storyId: string, eventIds: string[]): Promise<Record<string, unknown>> {
  const params = new URLSearchParams();
  for (const eventId of eventIds) params.append('event_id', eventId);
  return request<Record<string, unknown>>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/logs?${params.toString()}`,
    { method: 'DELETE' }
  );
}

export async function deleteLog(storyId: string, eventId: string): Promise<Record<string, unknown>> {
  return deleteLogs(storyId, [eventId]);
}

export async function getGlobalLogDetail(path: string): Promise<unknown> {
  const params = new URLSearchParams({ path });
  const data = await request<{ detail: unknown }>(
    `/admin/api/logs/detail?${params.toString()}`
  );
  return data.detail;
}

export async function deleteGlobalLogs(eventIds: string[]): Promise<Record<string, unknown>> {
  const params = new URLSearchParams();
  for (const eventId of eventIds) params.append('event_id', eventId);
  return request<Record<string, unknown>>(`/admin/api/logs?${params.toString()}`, {
    method: 'DELETE'
  });
}

export async function deleteGlobalLog(eventId: string): Promise<Record<string, unknown>> {
  return deleteGlobalLogs([eventId]);
}

export async function getModelRuns(storyId: string): Promise<FileInfo[]> {
  const data = await request<{ files: FileInfo[] }>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/model-runs`
  );
  return data.files;
}

export async function getIndex(storyId: string): Promise<IndexResponse> {
  return request<IndexResponse>(`/admin/api/stories/${encodeURIComponent(storyId)}/index`);
}

export async function getFile(storyId: string, path: string): Promise<string> {
  const params = new URLSearchParams({ path });
  const res = await fetch(`/admin/api/stories/${encodeURIComponent(storyId)}/file?${params}`);
  if (!res.ok) {
    throw new Error(await res.text());
  }
  return await res.text();
}

export async function runControl(
  storyId: string,
  action: string,
  body: Record<string, unknown> = {}
): Promise<Record<string, unknown>> {
  return request<Record<string, unknown>>(
    `/admin/api/stories/${encodeURIComponent(storyId)}/control/${action}`,
    {
      method: 'POST',
      body: JSON.stringify({ confirm: true, ...body })
    }
  );
}
