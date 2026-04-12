# Product Requirements Document: Automated Note Annotation

## 1. Overview

This document defines the product requirements for an automated annotation system that analyzes note content and generates AI-powered suggestions for internal links and tags. Suggestions are written into a dedicated, structured metadata section, keeping them clearly separated from authored content. The system is designed for a personal knowledge management (PKM) workspace with a hierarchical, namespace-based note structure.

## 2. Problem Statement

As a knowledge base grows, authors struggle to maintain a well-connected, consistently tagged note graph. Discovering relevant existing notes and applying consistent tags requires manual effort that is often skipped, resulting in fragmented knowledge and missed connections.

## 3. Goals

- Automatically surface relevant internal links and tags as suggestions whenever a note is saved.
- Keep AI-generated content strictly separated from author-written content.
- Give the author full control: suggestions are proposals, not automatic changes.
- Preserve author decisions (accepted/rejected) across future suggestion runs.
- Protect sensitive namespaces and PII from being sent to external AI providers.

## 4. Non-Goals

- Editing, rewriting, or correcting note body content.
- Spell-checking or grammar suggestions.
- Auto-applying any suggestion without an explicit author action.
- Cross-workspace or cross-repository linking.
- Rich editor UI (beyond a minimal trigger mechanism) in the initial release.

## 5. Users

Single-user: a knowledge worker maintaining a personal notes workspace. The system runs locally and has no multi-user or collaborative surface area.

## 6. Functional Requirements

### 6.1 Trigger

- The system is triggered each time the author saves a Markdown note.
- An optional continuous-watch mode supports batch processing outside of the editor.

### 6.2 Namespace Exclusion

- Certain namespaces (e.g., `people.*`) are permanently excluded from AI processing.
- Saving a file in an excluded namespace produces a logged skip event; no AI call is made.
- Excluded namespaces are configurable.

### 6.3 Privacy and Redaction

- Before any content is sent to an AI provider, a redaction pipeline scrubs PII: phone numbers, email addresses, and URLs are replaced with a configurable placeholder marker.
- Redaction patterns are configurable.

### 6.4 Change Detection

- A content hash of the note body and relevant metadata is computed on each save.
- If the hash is unchanged from the last processed run, the AI call is skipped and the existing suggestions are left intact.

### 6.5 Suggestion Generation

The system requests two types of suggestions from the AI provider:

1. **Internal links** — links to other notes in the workspace that are contextually relevant to the current note. Only notes that actually exist in the workspace index may be suggested; hallucinated targets are discarded.
2. **Tags** — both tags already in use across the workspace and new tags not yet in the catalog.

Each suggestion carries:
- A confidence score (0.0–1.0)
- A short reason string explaining why the suggestion was made
- A status field

### 6.6 Suggestion Schema

All AI output is written exclusively into a dedicated `ai_suggestions` metadata block. The top-level note metadata (including the `tags` field) is never modified during suggestion generation.

The `ai_suggestions` block contains:

- **Meta section**: generation timestamp, provider/model identity, spec version, and source hash.
- **Links section**: list of suggested internal links with target, anchor text, confidence, reason, and status.
- **Tags section**: two sub-lists — existing tags recommended for this note, and new tags proposed for the first time — each with confidence, reason, and status.

The `edits` field is permanently excluded from the schema.

#### Suggestion Status Values

| Status | Meaning |
|---|---|
| `proposed` | New suggestion awaiting author review |
| `accepted` | Author has accepted this suggestion |
| `rejected` | Author has rejected this suggestion |
| `applied` | Tag suggestion has been promoted to top-level metadata |
| `target_missing` | Previously proposed link target no longer exists in the workspace |

### 6.7 Confidence Threshold and Limits

- Only suggestions with confidence ≥ 0.50 are written.
- Per-namespace limits cap the number of link and tag suggestions written (see Section 11).

### 6.8 Merge Rules

When the system re-runs on a previously annotated note, the following rules govern how existing suggestions are handled:

1. **Accepted** suggestions are preserved permanently, even if the new AI run does not include them.
2. **Rejected** suggestions are never re-surfaced, regardless of confidence.
3. **Proposed** suggestions are replaced by the new AI output.
4. **Target-missing** status is set (not silently dropped) when a previously proposed link target no longer exists.
5. **Applied** suggestions remain in the block as an audit trail.

### 6.9 Tag Promotion (Apply Action)

1. Saving a note writes tag suggestions to `ai_suggestions` only.
2. The author explicitly runs a separate apply action to promote `accepted` tag suggestions into the top-level `tags` metadata field.
3. On promotion, the suggestion status changes to `applied` and the entry remains in `ai_suggestions`.
4. Both existing and new tags may be promoted.

### 6.10 Queue and Resiliency

The system maintains a persistent, FIFO-ordered, deduplicated work queue.

- Every save event is appended to the queue.
- If a file is already in the queue, it is removed from its current position and re-appended to the back with an updated modification timestamp.
- The worker processes only the front of the queue.
- **Pre-write conflict check**: before writing results, the system checks whether the file has been modified since it was enqueued. If it has, the result is discarded, the file is re-enqueued at the back with the new modification timestamp, and a `conflict_discards` counter is incremented.
- On successful processing, the file is removed from the queue.
- On AI provider errors, rate limits, or unavailability, the file remains at the front of the queue for retry. It is not removed.

### 6.11 Retry Policy

- Network failures, provider unavailability, and rate limit responses are eligible for retry.
- Retry uses exponential backoff with jitter (example schedule: 2s, 4s, 8s, 16s, 32s, up to 60s max).
- An optional maximum retry attempt cap is configurable; a value of 0 means unlimited.

### 6.12 Persistence

Queue state, the event log, the note index cache, and aggregated metrics must survive process restarts. All state is stored in a local directory within the workspace.

## 7. Non-Functional Requirements

| Requirement | Detail |
|---|---|
| **Performance** | Suggestion generation completes within 2–6 seconds for a typical note |
| **Determinism** | Output schema is strict; no free-form metadata mutation |
| **Safety** | Note content is never executed; excluded namespaces are never sent to the AI provider |
| **Idempotency** | Unchanged content produces no AI call and no write |
| **Durability** | Queue, index cache, and event log survive process restarts |
| **Auditability** | Provider and model identity and source hash are recorded with every suggestion batch |
| **Observability** | `conflict_discards` is the primary metric for evaluating whether debounce is needed |

## 8. Context Grounding for AI Suggestions

To generate high-quality suggestions, the AI provider is given the following context:

| Context | Description |
|---|---|
| Full note index | All note filenames, titles, namespaces, and modification times, sorted most-recently-modified first |
| Frontmatter schema | Known metadata keys and local conventions |
| Link graph | Existing internal links and backlink frequency |
| Tag catalog | All tags used across the workspace (frontmatter only; inline hashtags excluded) |
| Title/alias index | Mapping between filenames, frontmatter titles, and aliases |
| Section heading index | Common headings for anchor relevance |
| Policy constraints | Confidence thresholds, suggestion limits, status behavior, no-auto-apply rules |
| Prior suggestions | Existing `ai_suggestions` block and current statuses |
| Adjacent notes | For daily journal entries: full body content of the N preceding daily notes (configurable per namespace) |

The AI provider is strictly instructed:
- Only suggest links to notes present in the provided index.
- Include a confidence score and short reason for every suggestion.
- Never suggest edits to note body content.
- Output must conform to the defined schema.

## 9. Privacy and Security Requirements

- Files in excluded namespaces are never sent to an AI provider under any circumstances.
- PII redaction runs before any content is included in an AI request.
- Request and response logs are stored locally and subject to configurable retention.
- API credentials are never written into note files or the repository.
- An allowlist of directories eligible for AI processing is supported.

## 10. Observability

### 10.1 Event Log

An append-only event log records all system activity with timestamps. Logged events include:

- `saved` — note save detected
- `enqueued` — file added to queue
- `skipped_excluded_namespace` — file skipped due to namespace exclusion
- `conflict_discarded` — result discarded due to file modification during processing
- `retry_needed` — AI call failed; file retained at queue front
- `processed` — file successfully processed and removed from queue

Log entries are retained for a configurable number of days (default: 14).

### 10.2 Metrics

- Suggestion counts by type (links, existing tags, new tags)
- Acceptance and rejection rates by type
- `target_missing` count (false-positive link signal)
- Average confidence of accepted suggestions
- Median latency per save event
- `conflict_discards` count (primary debounce signal)
- AI provider errors and retry counts
- Files skipped by namespace exclusion

### 10.3 Report

A report command summarizes the above metrics over the retention window. Example output shape:

```
AI Suggestions Report — last 14 days
─────────────────────────────────────
Files processed:        142
Files skipped:           23  (excluded namespace)
Conflict discards:        8  (debounce signal: low)
AI errors / retries:      3

Links suggested:        312   accepted: 89 (29%)  rejected: 41 (13%)
Tags suggested:         198   accepted: 71 (36%)  rejected: 28 (14%)

Avg confidence (accepted links): 0.74
Avg confidence (accepted tags):  0.68
Avg latency per save:          3.2s
```

## 11. Per-Namespace Policies

Behavior can be overridden per namespace. The following defaults are recommended:

| Namespace | Excluded | Max links | Max existing tags | Max new tags | Adjacent notes context |
|---|---|---|---|---|---|
| `people.*` | Yes | — | — | — | — |
| `daily.journal.*` | No | 8 | 3 | 2 | 3 preceding daily notes |
| `projects.*` | No | 12 | 5 | 3 | — |
| `reference.*` | No | 8 | 5 | 2 | — |
| `technology.*` | No | 10 | 5 | 3 | — |

## 12. Error Handling

| Condition | Behavior |
|---|---|
| AI provider call fails | Preserve existing `ai_suggestions`; record error in `meta.last_error` |
| AI output invalid/malformed | Reject payload; optionally retry once with a stricter prompt |
| Note parsing fails | No write; emit structured error log entry |
| Note index unavailable | Run in limited mode using filename/title heuristics only |
| AI provider unavailable or rate-limited | Keep file at front of queue; log `retry_needed` |
| Process crash during run | Recover queue and events from persistent state on restart; resume with same front-of-queue file |
| Pre-write conflict detected | Discard result; re-enqueue at back; increment `conflict_discards` |

## 13. Configuration

The system is driven by a configuration file. Configurable parameters include:

- AI provider selection and model
- Request timeout
- Confidence threshold
- Per-type suggestion limits (globally and per namespace)
- Status preservation rules
- Workspace root path and include/exclude globs
- Privacy: excluded namespaces, redaction patterns, and redaction marker
- Queue storage directory and event log file
- Event retention period
- Index cache flush interval
- Retry policy (enabled/disabled, backoff schedule, max attempts)
- Per-namespace policy overrides

## 14. Implementation Milestones

### M1 — Foundation
- Note parsing and atomic metadata write
- Content hash computation
- Note index build with modification-time-based caching and lazy flush
- Persistent FIFO deduplicating queue with conflict detection
- Append-only event log
- Full configuration schema
- Editor save hook wired to the processing entrypoint

**Verification**: saving a file produces correct queue and event log updates.

### M2 — Data Sanitization
- Namespace exclusion check
- Redaction pipeline (phone, email, URL)
- `skipped_excluded_namespace` event logging
- Unit tests for redaction and exclusion logic

**Verification**: excluded-namespace notes never reach AI prompt assembly; redacted content confirmed in dry-run output.

### M3 — AI Integration
- AI provider abstraction supporting multiple providers via configuration
- Prompt assembly: full mtime-sorted index, tag catalog, parsed note, prior suggestions, adjacent daily notes
- Structured output schema enforcement (links and tags only)
- Output validation: confidence range, link target existence, tag format, threshold filter, deduplication
- Suggestion merge with status preservation
- Atomic metadata write
- Startup health check; `meta.last_error` on failure

**Verification**: saving a file populates `ai_suggestions` with links and tags.

### M4 — Resilience
- Retry with exponential backoff and `retry_needed` events
- Per-namespace policy overrides in prompt assembly
- Adjacent daily notes context for journal namespace
- Lazy index flush on configurable timer
- Dry-run flag for safe testing

**Verification**: AI unavailability handled gracefully; namespace policies applied correctly.

### M5 — Apply Action and Reporting
- Apply command to promote `accepted` tag suggestions into top-level metadata
- Status transition to `applied` on promotion
- Editor task keybinding wired to apply command
- Report command with all metrics including `conflict_discards`

**Verification**: accepting a tag suggestion and running apply updates top-level metadata correctly.

### M6 — Quality Tuning
- Evaluate suggestion quality against a representative note corpus
- Tune per-namespace confidence thresholds and limits based on report data
- Decide on debounce implementation based on `conflict_discards` signal
- Evaluate provider quality in practice
- Assess continuous-watch mode for batch/non-editor workflows

## 15. Open Questions

1. Should link recommendations have a hard cap per namespace, and are the defaults in Section 11 correct?
2. Should per-namespace policy overrides be supported beyond the five significant namespaces listed?
3. After M6 evaluation: is debounce warranted based on `conflict_discards` data?
