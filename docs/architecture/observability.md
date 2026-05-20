# Observability

The agent loop has silent failure modes: lore drift, weak choices, actor bleed,
tool context leaks, provider-specific prompt attractors, sidecar timeouts, and
Honcho witness mistakes. Instrumentation is part of the design, not polish.

## Backstage Transcript

The admin "Behind The Scenes" view normalizes logs into a live office-chat
transcript:

- system mentions such as `@Writer`, `@Narrator`, `@Scenarist`, `@LoreExpert`,
  and `@Voice`;
- model responses under their role names;
- local read-tool calls and results;
- provider/model/reasoning/service-tier metadata;
- pending/completed/failed status;
- artifact and detail links.

Raw JSONL/detail logs remain the forensic source of truth. The transcript is an
admin-friendly projection over those logs, with `visibility` defaulting to
`admin_only`.

## Required Log Fields

Per model/tool event:

- `time`;
- story id, seed id, packet id, node id, turn, and run id when available;
- role and speaker;
- kind and status;
- source event and source event id;
- provider type, model provider, model, reasoning level, and service tier when
  available;
- scoped agent workspace root and visible paths for model tool events;
- artifact paths and detail path;
- redacted prompt/result preview.

## Metrics

Track at minimum:

- token usage by role and provider;
- latency by stage: context assembly, Writer, Scenarist, LoreExpert, Narrator,
  TTS, Honcho commit;
- tool-call count and rejected path attempts;
- Writer refresh frequency after in-packet runway exhaustion;
- actor-call count per turn;
- lore-fidelity verifier failures;
- scene/session commit duration and timeout rate;
- choice pick vs free-text ratio.

## Traces

Each turn should have one root trace with spans for:

- loading story state;
- following packet target or triggering Writer refresh;
- staging agent workspace files;
- model/tool calls;
- Narrator rollout;
- TTS;
- Honcho context/commit;
- checkpoint save.

Trace IDs should propagate Go -> Python sidecar -> Honcho where possible.

## Quality Signals

Useful automated checks:

- seed verifier rejects missing runway shape;
- lore-fidelity verifier rejects contract contradiction;
- Narrator quality check flags exposition-heavy or generic scenes;
- backstage scan flags raw repo path attempts;
- admin prompt-log inspection confirms models read `lorepack/`, not raw
  `lore/packs/...`.

## Alerting

Critical:

- bridge sidecar down;
- Honcho commit timeout rate above threshold;
- model provider outage;
- repeated agent workspace path rejections inside one run.

Warning:

- turn latency over budget;
- lore-fidelity rejection rate climbing;
- actor invocation requests with missing cards;
- TTS failures;
- repeated Writer refreshes caused by malformed packet choices.

## Privacy And Retention

Player input and generated prose are story content. Keep production logs
redacted by default, retain detailed local artifacts for dev/admin workflows,
and make deletion/archive behavior explicit when player-facing accounts exist.
