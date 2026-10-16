# Journal Recall â€” Technical Specification

**Status:** Draft  
**Product type:** B2C web application  
**Specification scope:** Requirements supplied by the product owner  

## 1. Product Objective

Journal Recall enables people to maintain a portable, human-readable journal and later search or question it to determine what happened and when.

The product has three principal capabilities:

1. **Journal Entry** â€” create and store journal entries.
2. **Journal Recall** â€” search entries and ask questions about them.
3. **Journal Detection Rules and Statistics** â€” classify events or incidents mentioned in entries so that occurrences can be counted and analysed.

## 2. Core Principles

- Every journal entry must exist as a raw, human-readable Markdown file on the filesystem.
- Raw Markdown files are the source of truth.
- The canonical indexed representation must be Markdown or structured JSON.
- Search data and indexes must be reproducible from the raw Markdown files.
- Retrieval must not depend on embeddings alone.
- Search results and generated answers must remain traceable to journal evidence.
- Historical information must not be silently overwritten.
- Chunks must follow semantic boundaries rather than arbitrary token counts.

## 3. Functional Scope

### 3.0 MVP Scope

The MVP must provide four user-facing sections:

1. **Journal Entry** â€” create, save and automatically process a journal entry so that it is ready for retrieval.
2. **Journal Recall** â€” submit a natural-language prompt and receive an LLM-generated answer grounded in journal evidence.
3. **Journal Search** â€” perform conventional full-text search across all raw Markdown entries using the backend search capability also available to the recall pipeline.
4. **Journal Statistics** â€” view statistics produced from detection rules preconfigured by a developer in SQLite.

The MVP does not include an administration UI for creating or managing detection rules.
Authentication and account-management functionality are outside the MVP.

### 3.1 Journal Entry

#### JE-001: Entry form

The web application must present a journal-entry form with the following fields.

| Field | Visibility | Requirement |
|---|---|---|
| Title | User-visible | Text title for the entry. |
| Date | User-visible | Optional. Defaults to today. The user may select the day represented by the entry. |
| Description | User-visible | Free-text Markdown content. |
| Tags | User-visible | Comma-separated tags with autocomplete support. Used for search. |
| Internal tags | Hidden | System-generated metadata derived from the entry. |

#### JE-002: Raw Markdown persistence

Saving an entry must create a raw `.md` file on the filesystem.

- The title must appear as a top-level Markdown heading.
- The description must be stored as Markdown without removing the user's Markdown intent.
- The file must remain directly readable by a human without the application.
- Raw Markdown files are the authoritative source of journal content.

#### JE-003: File location and naming

The server must assign each user an immutable user ID and store raw Markdown files under the directory associated with that ID. Mutable login names must not be used as filesystem identifiers.

Within that directory:

- entries must be grouped into daily folders based on the UTC creation date;
- the filename must include the UTC date and time at which the entry was first submitted and created; and
- the creation time must be stored in UTC but displayed to the user using the browser's local time.

#### JE-004: Markdown front matter

Every raw Markdown file must begin with YAML front matter containing authoritative portable metadata.

```yaml
---
schema_version: 1
id: "019c45a6-3e22-7d31-a794-1d12dde6c123"
title: "Completed payment integration"
journal_date: "2026-08-12"
created_at: "2026-08-12T13:42:18Z"
updated_at: "2026-08-12T14:10:03Z"
timezone: "Europe/Malta"
tags:
  - work
  - payments
status: active
---
```

The required fields are:

- `schema_version`;
- `id`, stored as a UUIDv7;
- `title`;
- `journal_date`;
- `created_at`, stored as an immutable UTC timestamp;
- `updated_at`, stored as a UTC timestamp;
- `timezone`, captured from the browser when the entry is created;
- `tags`, containing user-defined tags; and
- `status`, with the value `active` or `deleted`.

Generated summaries, embeddings, entities and internal tags must not be stored in the raw Markdown file because they are rebuildable artefacts.

#### JE-005: Content restrictions

Journal entries must not contain uploaded or embedded images, video or other media. Media may only be represented by a link in the Markdown description.

#### JE-006: Canonical indexed representation

In addition to the raw Markdown file, the system must store a canonical representation in the database as either:

- Markdown; or
- structured JSON.

#### JE-007: Metadata enrichment

When an entry is saved, its content must be passed through an LLM to produce retrieval-oriented metadata.

The enrichment output must include:

- a highly compact summary of the entry; and
- internal tags intended to improve later search and recall.

Internal tags must not be shown as an editable journal-entry field.

#### JE-008: Entity organisation

The system must recognise and organise information around these initial entity types:

- Projects
- People
- Brands
- Features
- Releases
- Tickets
- Commits
- Merges
- Pull Requests
- Companies
- Systems
- Decisions
- Incidents
- Tasks
- Concepts
- Dates

#### JE-009: Entry viewing and metadata

When a raw journal entry is viewed, its tags and generated metadata may be displayed around the raw-file content without becoming part of the user-authored body.

#### JE-010: Entry editing and reprocessing

A user must be able to edit a raw Markdown entry after creation. After an edit, all linked and derived data for that entry must be regenerated from the updated raw file.

Editing must preserve the entry ID, original filename and `created_at` timestamp. It must update `updated_at`.

The system must retain one immediately previous version of the raw Markdown file for recovery. A full version-history interface is not required for the MVP.

#### JE-011: Entry deletion state

Deletion must be implemented as soft deletion by changing the front-matter status to `deleted` and adding a UTC `deleted_at` timestamp.

- Deleted files must remain on disk and be restorable.
- Normal search, recall and statistics must exclude deleted entries by default.
- Search and recall may include deleted entries when explicitly requested.
- When a deleted entry is displayed or used as evidence, it must be clearly labelled `Deleted`.
- Permanent deletion is outside the MVP.

#### JE-012: AI-independent saving and resynchronisation

Saving the authoritative raw Markdown file must not depend on LLM, embedding or other AI services being available.

The system must:

1. write the Markdown file atomically;
2. record the entry as requiring processing;
3. return a successful save once the raw file is safely persisted;
4. process enrichment and indexing separately; and
5. allow failed or pending entries to be resynchronised later from their raw Markdown files.

Processing must expose the states `pending`, `processing`, `ready` and `failed`. Full-text search must become available without waiting for AI enrichment.

### 3.2 Document Indexing

#### IDX-001: Indexed representations

Each canonical document must have the following indexed representations:

- document summary;
- section-level chunks;
- small factual chunks;
- title, user tags, internal tags and entities;
- exact text for keyword search; and
- embeddings for semantic search.

#### IDX-002: Semantic chunking

Chunks must align with semantic boundaries such as headings, messages and decisions. They must not be created using an arbitrary fixed window such as 500 tokens.

Every chunk must include enough parent context to remain understandable when retrieved independently.

#### IDX-003: Rebuildability

All search data, summaries, internal tags, entities, chunks, embeddings and other derived indexing artefacts must be safely rebuildable from raw Markdown files.

Automated full reconstruction through a cron job or agent execution is not required for the MVP.

#### IDX-004: Model deployment and provenance

The system must support locally hosted LLM and embedding models.

If an external model provider is configured:

- provider training on journal data must be disabled;
- provider retention must be disabled where supported; and
- the provider name, model identifier, prompt version and processing timestamp must be stored with each generated artefact.

Embeddings and other derived representations must be treated as sensitive journal data.

### 3.3 Journal Recall

#### RET-001: Recall interface

The recall page must allow a user to submit a natural-language prompt. The backend and LLM must use the indexed journal data as needed to generate an answer about what happened and when.

The answer must include references to the applicable raw Markdown files.

#### RET-002: Hybrid retrieval

Retrieval must combine:

1. full-text/BM25 search for exact names, identifiers and phrases;
2. vector search for semantic similarity;
3. metadata filters, including project, date, source, author and permissions;
4. graph traversal for relationships; and
5. reranking to select the most relevant evidence.

The system must not depend on embeddings alone because exact identifiers, uncommon names and negated facts may not be retrieved reliably through semantic similarity.

#### RET-003: Query-aware retrieval

Before retrieval, an LLM must transform the user's question into a structured search plan with this shape:

```json
{
  "intent": "find current deployment procedure",
  "keywords": ["Kwikky", "deployment"],
  "entities": ["Kwikky"],
  "filters": {
    "type": ["runbook", "decision"]
  },
  "time_preference": "latest",
  "queries": [
    "current Kwikky deployment procedure",
    "Kwikky production release runbook"
  ]
}
```

The retrieval pipeline must then:

1. retrieve broadly;
2. merge duplicate results;
3. rerank the merged candidates; and
4. expand neighbouring sections only when necessary.

#### RET-004: Time and truth

The knowledge model must support:

- `valid_from`;
- `valid_until`;
- `supersedes` relationships;
- status values `draft`, `current` and `deprecated`;
- conflict detection;
- source-authority ranking; and
- preference for the latest applicable version.

The system must never silently overwrite historical decisions.

#### RET-005: Evidence-grounded answers

Every generated answer must include:

- a direct answer;
- source links;
- relevant excerpts;
- relevant dates; and
- confidence or unresolved conflicts.

If the retrieved evidence is insufficient, the system must state that it cannot answer from the available journal evidence. It must not fill the gap using the model's general memory.

#### RET-006: Ranking and result limit

Recall evidence must be ranked by best match to the user's request. A recall operation must use no more than 10 evidence sources, deduplicated by journal entry. When more than 10 similarly relevant sources remain or the request is too broad, the application must ask the user to refine the search.

The recall page must return one consolidated answer with this structure:

1. direct answer;
2. evidence list containing the raw filename, journal date, relevant excerpt and deleted status;
3. confidence; and
4. unresolved conflicts, when present.

Every journal-derived factual claim must be supported by a citation. The system must abstain when sufficiently relevant evidence is unavailable and must not use general model knowledge to answer journal-specific facts.

#### RET-007: Recall-response export

The user must be able to export a prompt and its generated recall response as a PDF, including its evidence references.

### 3.4 Journal Search

#### SRCH-001: Full-text search page

The application must provide a dedicated search page for conventional full-text search across all raw Markdown journal files belonging to the user.

The search page must use the backend full-text search capability already implemented for use by the recall pipeline.

SQLite FTS5 must provide the MVP full-text index. Search must:

- cover title, Markdown body and user tags;
- support exact phrases using quotation marks;
- support date-range and tag filters;
- rank results using BM25 with title matches weighted above body matches;
- return 20 results per page;
- display highlighted matching excerpts;
- exclude deleted entries by default;
- provide an option to include deleted entries; and
- open the raw journal view when a result is selected.

#### SRCH-002: Raw-file export

While viewing search results or raw journal files, the user must be able to export either one selected raw Markdown file or a selected group of raw Markdown files.

### 3.5 Journal Detection Rules and Statistics

#### DET-001: MVP rule configuration

For the MVP, detection rules must be manually predefined by a developer in configuration stored in SQLite.

The MVP must not include a user or administrator interface for creating or managing rules.

#### DET-002: Rule capability

Each configured rule must identify a point or event mentioned in a journal entry.

Example rule:

> Every time I mention closing a ticket.

Rules must use a structured, versioned configuration with this logical shape:

```json
{
  "id": "closed-ticket",
  "name": "Tickets closed",
  "description": "Detect when the user states that they closed a ticket.",
  "detection_prompt": "Does the entry state that the author completed or closed a work ticket?",
  "enabled": true,
  "minimum_confidence": 0.8,
  "version": 1
}
```

Rules are global and developer-managed. They must be evaluated after each create or edit. A rule-version change must permit all active entries to be reprocessed.

#### DET-003: Rule evaluation

When a journal entry contains a point that satisfies a detection rule, the system must add a reference to that occurrence to a list associated with the rule.

Each occurrence must store its supporting excerpt and model confidence. Multiple occurrences may be recorded for one entry only when they represent distinct events. Processing must be idempotent and must not count the same occurrence more than once.

#### DET-004: Statistics

The application must provide a statistics section where the user can view statistics generated from the occurrence lists produced by the configured detection rules.

Detection and categorisation may use sentiment analysis and semantic analysis.

For each rule, the MVP statistics section must provide:

- total occurrence count;
- daily, weekly and monthly counts; and
- access to the supporting journal entries behind each count.

Statistics must be grouped using `journal_date` and displayed using the browser's timezone. Deleted entries must be excluded. Editing, deleting or restoring an entry must recalculate the affected occurrences and statistics. Statistics must be rebuildable from raw Markdown files and detection-rule configuration.

## 4. Data Ownership, Storage and Export

### DATA-001: Server storage

Except for supported exports, application data must be stored on the server. Raw Markdown files must be stored under directories associated with each user's immutable user ID.

### DATA-002: Source of truth

Raw Markdown files are the source of truth. The system must be designed so that search data and indexes can later be recreated using only those raw files.

### DATA-003: Rebuildable artefacts

All artefacts, including statistics, must be safely rebuildable from raw Markdown files and developer-managed detection-rule configuration.

### DATA-004: Supported exports

The MVP must support only these exports:

- one selected raw Markdown file;
- a selected group of raw Markdown files; and
- a recall prompt and generated response as a PDF with evidence references.

## 5. Conceptual Data Objects

The specification requires the following conceptual objects. Their physical schema is not defined here.

### Journal Entry

- Title
- Journal date
- Description
- User tags
- Internal tags
- Raw Markdown file reference
- Canonical representation
- Compact summary
- Status and temporal validity metadata
- UTC creation timestamp
- Deleted state
- One previous raw-file version

### Entity

- Entity type
- Entity value or identity
- Relationships to entries, chunks or other entities

### Indexed Chunk

- Entry reference
- Parent section context
- Chunk content
- Chunk granularity or type
- Associated entities and metadata
- Exact-text index representation
- Embedding representation

### Detection Rule

- Rule definition
- SQLite configuration
- References to detected occurrences
- Statistics derived from occurrences

### Evidence Reference

- Source entry or chunk
- Relevant excerpt
- Relevant date
- Link to the source

## 6. End-to-End Behaviour

### 6.1 Save and index an entry

1. The user enters a title, date, Markdown description and optional tags.
2. The application saves a raw human-readable Markdown file.
3. The application returns save success without waiting for AI processing.
4. The application stores a canonical Markdown or structured-JSON representation.
5. The LLM generates a compact summary and internal tags when available.
6. The system identifies supported entities.
7. The system creates semantic section and factual chunks with parent context.
8. The system indexes exact text, metadata, entities and embeddings.
9. The system evaluates journal detection rules and records matching occurrences.

### 6.2 Recall journal information

1. The user submits a search or question.
2. The LLM creates a structured search plan.
3. The system performs hybrid retrieval.
4. The system merges duplicates, reranks candidates and expands adjacent context when needed.
5. The system evaluates time, status, authority and conflicts.
6. The system returns an evidence-grounded answer or states that the available evidence is insufficient.

### 6.3 Edit and reprocess an entry

1. The user edits an existing raw Markdown entry.
2. The system saves the updated raw file without changing its original UTC creation identity.
3. The system regenerates all linked and derived data from the updated raw file.
4. The updated entry becomes available to full-text search, recall and configured detection rules.

### 6.4 Delete an entry

1. The user deletes an entry.
2. The system records the entry's deleted state but preserves access to it.
3. Search or recall may still use the entry.
4. Any answer citing it must clearly label it as deleted evidence.

## 7. Acceptance Criteria

### Journal entry

- A user can save an entry containing a title and Markdown description.
- Omitting the date stores the entry against today's date.
- Selecting another date stores the entry against that selected journal date.
- Tags support autocomplete and comma-separated entry.
- Saving creates a directly readable `.md` file.
- The file is stored under the user's immutable user-ID directory and a UTC daily folder.
- The filename contains the entry's UTC creation date and time.
- Creation times are displayed using the browser's local time.
- The stored Markdown presents the title as a top-level heading.
- Saving produces a canonical database representation, compact summary and hidden internal tags.
- Every file contains the required YAML front matter and UUIDv7 entry ID.
- Saving succeeds when AI services are unavailable and the entry can be resynchronised later.
- Full-text search does not wait for AI enrichment.
- Media cannot be uploaded or embedded, but links may be entered.
- Editing an entry regenerates all linked and derived data.
- Editing retains one previous raw-file version.
- A deleted entry remains retrievable and is clearly labelled as deleted when cited.

### Indexing and recall

- Exact identifiers and phrases can be found through full-text search.
- Semantically related content can be found without requiring exact wording.
- Results can be filtered using supported metadata.
- Entity relationships can contribute to retrieval.
- Retrieved candidates are reranked before answering.
- A retrieved chunk is understandable without requiring an arbitrary missing preceding chunk.
- Questions about what happened and when return evidence, excerpts and dates.
- Recall answers reference the applicable raw Markdown files.
- Recall produces one consolidated answer using at most 10 deduplicated evidence sources and asks for refinement when appropriate.
- A recall prompt, response and evidence references can be exported as PDF.
- Insufficient evidence produces an explicit non-answer rather than an unsupported answer.
- Historical decisions remain available after being superseded.

### Full-text search

- A dedicated page performs conventional full-text search across the user's raw Markdown files.
- Search uses SQLite FTS5 and the backend capability shared with the recall pipeline.
- Search supports exact phrases, tag and date filters, BM25 ranking, highlighted excerpts and 20-result pagination.
- A user can export one selected raw file or a selected group of raw files while viewing them.

### Detection rules

- Detection rules are manually configured by a developer in SQLite.
- The MVP contains no rule-management UI.
- Matching journal content creates an occurrence reference associated with that rule.
- The user can view statistics generated from recorded occurrences.
- Statistics show total, daily, weekly and monthly counts with access to supporting entries.
- Statistics exclude deleted entries and are recalculated after entry changes.

### Rebuildability

- Raw Markdown files remain the source of truth.
- All derived search and indexing artefacts can be recreated from the raw files.
- Automated scheduled or agent-driven reconstruction is not required for the MVP.

### Privacy and model support

- Locally hosted LLM and embedding models are supported.
- External provider training on journal data is disabled.
- Provider, model, prompt version and processing timestamp are recorded for generated artefacts.

### Performance

- Persisting a raw journal entry must complete within 500 ms at the 95th percentile, excluding asynchronous enrichment.
- Full-text search must complete within 500 ms at the 95th percentile for a dataset of up to 100,000 entries.
- Recall must produce a response within 15 seconds at the 95th percentile.
- Background processing of an entry must complete within 30 seconds at the 95th percentile when the configured model is available.
- A journal entry must not exceed 100 KB.
- A journal entry must not contain more than 100 user tags.
- Recall must use no more than 10 evidence sources.

## 8. Explicitly Unspecified

The following are intentionally not defined in this specification because they were not supplied:

- implementation language and framework;
- search-engine implementation beyond the specified SQLite configuration and backend search requirements;
- specific LLM, embedding model and reranking model;
- authentication and account lifecycle, which are outside the MVP;
- deployment topology;
- user roles and permissions;
- UI design beyond the named fields and page;
- API contracts;
- the filename portion other than the required UTC creation date and time;
- statistics visual design.

## 9. Order of Execution

Implementation must use independently runnable vertical milestones. Each milestone must include its required user interface or API, filesystem behaviour, database changes, processing logic and automated tests. The implementation must not build every technical layer separately before delivering an end-to-end capability.

### 9.1 Milestone 1 â€” Standalone Journal Entry Service (Super MVP)

This is the first and highest-priority deliverable. It must run as a useful standalone service before the recall, search and statistics pages are built.

The service must provide:

1. the journal-entry form and submission endpoint;
2. validation of title, journal date, Markdown description and user tags;
3. UUIDv7 entry identity and immutable user-ID storage isolation;
4. YAML front matter;
5. UTC daily directory and filename creation;
6. atomic raw Markdown persistence;
7. canonical SQLite persistence;
8. SQLite FTS5 indexing;
9. compact LLM-generated summary and internal tags;
10. entity extraction and relationship storage;
11. semantic section and factual chunk generation;
12. locally generated embeddings and vector indexing;
13. evaluation of developer-configured detection rules;
14. processing status, retry and manual resynchronisation; and
15. viewing and editing an entry, including one previous raw-file version.

Every successfully saved entry must be processed to the full extent required by the later retrieval pipeline. An entry is retrieval-ready only when its processing status is `ready` and all required derived artefacts have been generated and indexed.

Raw-file persistence must remain independent of AI availability. If model processing is unavailable, the save must succeed with status `pending` or `failed`, and the service must allow processing to resume later until the entry becomes `ready`.

The Super MVP does not require a recall page, search page or statistics page. Its database and indexes must nevertheless contain everything those later milestones require.

The milestone is complete only when an automated integration test can create a journal entry, wait for it to become `ready`, and verify the raw file, front matter, canonical record, FTS record, chunks, entities, embeddings, model provenance and rule occurrences.

### 9.2 Milestone 2 â€” Journal Lifecycle

Add and verify:

- entry viewing;
- editing and complete regeneration of derived data;
- retention of exactly one previous raw-file version;
- soft deletion and restoration;
- deleted-entry filtering; and
- idempotent reprocessing without duplicated entities, chunks or rule occurrences.

### 9.3 Milestone 3 â€” Full-Text Search and Export

Build the dedicated FTS5 search page using the index already populated by the standalone Journal Entry service. Add phrase search, filters, BM25 ranking, highlighted excerpts, pagination, deleted-entry controls and raw-file export.

### 9.4 Milestone 4 â€” Journal Recall

Build query planning, hybrid retrieval, vector search, relationship traversal, reranking, evidence deduplication, grounded answer generation, abstention and PDF export. Reuse the artefacts created during Milestone 1 rather than introducing a separate ingestion pipeline.

### 9.5 Milestone 5 â€” Statistics

Build total, daily, weekly and monthly statistics over the detection occurrences already produced during entry processing. Add evidence navigation and correct recalculation after edits, deletion and restoration.

### 9.6 Execution Gate

Before starting the next milestone:

- all requirements assigned to the current milestone must have automated tests;
- relevant unit and integration tests must pass;
- the requirement-to-test traceability matrix must be updated;
- no required data may exist only in test fixtures or temporary code; and
- any failed, deferred or changed requirement must be explicitly recorded.

## 10. Zero-Cost Development Infrastructure

The development environment must be capable of running locally without paid services. Manual installation and configuration are acceptable. The architecture must keep each component replaceable so hosted infrastructure can be introduced later without changing the journal file contract.

### 10.1 Required Local Components

| Capability | Development requirement |
|---|---|
| Web application | One locally running web application containing the UI and backend API, or separate local UI and API processes. The implementation framework remains open. |
| Authoritative files | A local filesystem directory mounted or configured as the journal-data root. |
| Relational storage | One local SQLite database file. Enable foreign keys and use migrations. |
| Full-text search | SQLite FTS5 in the same database. |
| Vector search | A locally installed SQLite-compatible vector extension, such as `sqlite-vec`, or an equivalent local vector index. |
| Relationship traversal | Normal SQLite entity and relationship tables for the MVP; a separate graph database is not required. |
| LLM execution | A locally hosted model runtime, such as Ollama or `llama.cpp`, exposed behind the application's model-provider interface. |
| Embeddings | A local embedding model invoked through the same runtime or a dedicated local embedding process. |
| Background processing | An in-process worker or separate local worker using a SQLite-backed job table. A message broker is not required for development. |
| PDF generation | A local server-side HTML-to-PDF or document-to-PDF library when the recall-export milestone begins. |
| Tests | Temporary filesystem directories, an isolated SQLite database and deterministic fake model adapters for automated tests. |

### 10.2 Development Topology

The minimum development topology is:

```text
Browser
  -> Local web application/API
       -> Journal filesystem
       -> SQLite + FTS5 + vector extension
       -> Local background processor
       -> Local LLM and embedding runtime
```

All components may run on one development machine. Docker is optional rather than required. Direct manual installation is acceptable and may simplify access to local models and filesystem data.

### 10.3 Local Directory Layout

The development environment should use configurable relative roots following this logical structure:

```text
data/
  journal.sqlite
  journals/
    {immutable_user_id}/
      YYYY-MM-DD/
        {utc_timestamp}_{entry_id}.md
        versions/
          {utc_timestamp}_{entry_id}.previous.md
  models/
  exports/
```

The exact physical paths must be configurable through development environment settings. Database records must not depend on machine-specific absolute paths.

### 10.4 Local Model Requirements

The developer must be able to configure:

- local model endpoint;
- LLM model identifier;
- embedding model identifier;
- request timeout;
- context limit;
- generation parameters; and
- model-processing concurrency.

The application must access models through interfaces rather than calling a particular local runtime directly from business logic. A deterministic fake implementation must be available for tests.

The chosen local models must support the structured outputs required for summaries, entities, chunks, query plans and detection results. Model downloads and execution must not require a paid account or API key.

### 10.5 Development Configuration

Development configuration must include at least:

- immutable development user ID;
- journal-data root;
- SQLite database path;
- vector-extension location or vector-index configuration;
- model-provider type (`local`, `external` or `fake`);
- local model endpoint and model identifiers;
- processing retry count and timeout;
- maximum entry size and tag count; and
- export directory.

Secrets, provider keys and machine-specific paths must not be committed to source control. A checked-in example configuration must document every required setting.

### 10.6 Infrastructure Not Required for the Development MVP

The zero-cost development environment does not require:

- cloud hosting;
- managed databases;
- object storage;
- a graph database;
- a dedicated search server;
- a message broker;
- Kubernetes;
- paid LLM or embedding APIs;
- distributed tracing infrastructure; or
- automated production deployment.

These components may be introduced later only when scale, multi-machine deployment or operational requirements justify them.
