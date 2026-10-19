# Requirement / Test Traceability

Updated as each vertical slice is completed. See `docs/decisions/` for the reasoning behind any scoping choices noted below.

## Milestone 1, Slice 1 — Entry-Save Endpoint (raw Markdown only)

Decision record: `docs/decisions/0001-slice1-entry-save.md`.

| Requirement | Description | Test(s) | Status |
|---|---|---|---|
| JE-001 | Entry form fields and validation (title, journal date, description, tags) | `internal/journal/validate_test.go`: `TestValidateAndNormalize_HappyPath`, `TestValidateAndNormalize_MissingTitle`, `TestValidateAndNormalize_MissingDescription`, `TestValidateAndNormalize_JournalDateDefaultsUsingClientTimezone`, `TestValidateAndNormalize_ExplicitJournalDate`, `TestValidateAndNormalize_RejectsInvalidCalendarDate`, `TestValidateAndNormalize_RejectsMalformedJournalDate`, `TestValidateAndNormalize_TagsTrimmedDedupedEmptyDropped`, `TestValidateAndNormalize_RejectsTooManyTags`, `TestValidateAndNormalize_AllowsExactlyMaxTags`; `internal/httpapi/entries_test.go`: `TestCreateEntry_HappyPath`, `TestCreateEntry_ValidationFailure_NoFileCreated` | Complete |
| JE-002 | Raw Markdown persistence; title as top-level heading; description preserved verbatim | `internal/journal/render_test.go`: `TestRender_FormatAndFields`, `TestRender_DescriptionPreservedByteForByte`; `internal/journal/store_test.go`: `TestSave_HappyPath` | Complete |
| JE-003 | File location and naming: immutable user-ID directory, UTC daily folder, UTC timestamp filename | `internal/journal/store_test.go`: `TestSave_HappyPath` (asserts exact relative path) | Complete |
| JE-004 | YAML front matter with all required fields (schema_version, id/UUIDv7, title, journal_date, created_at, updated_at, timezone, tags, status) | `internal/journal/render_test.go`: `TestRender_FormatAndFields`; `internal/journal/store_test.go`: `TestSave_HappyPath`; `internal/journal/validate_test.go`: `TestValidateAndNormalize_RequiresTimezone`, `TestValidateAndNormalize_RejectsLocalTimezone`, `TestValidateAndNormalize_RejectsGarbageTimezone` | Complete |
| JE-005 | Content restrictions (no embedded/uploaded media) | `internal/journal/validate_test.go`: `TestValidateAndNormalize_RejectsEmbeddedImage`; `internal/httpapi/entries_test.go`: `TestCreateEntry_ValidationFailure_NoFileCreated/embedded_image` | Complete, narrowed scope (Markdown image syntax only — see ADR 0001) |
| JE-012 | AI-independent atomic save; return success once raw file persisted | `internal/journal/store_test.go`: `TestSave_HappyPath`, `TestSave_RejectsOversizedEntry`, `TestSave_AllowsExactlyMaxSize`; `internal/httpapi/entries_test.go`: `TestCreateEntry_OversizedEntry_NoFileCreated` | Partial — raw persistence is complete and fully AI-independent (no model calls exist in this slice); durable `pending/processing/ready/failed` tracking and resync require a later slice's SQLite job table (see ADR 0001) |
| DATA-001 | Server storage under immutable user-ID directories | `internal/journal/store_test.go`: `TestSave_HappyPath` | Complete |
| DATA-002 | Raw Markdown files are the source of truth | Structural — no derived index exists yet to contradict this; verified by all `internal/journal` tests reading back only the raw file | Complete |

### Not yet started (future slices)

JE-006 (canonical SQLite/JSON representation), JE-007 (LLM enrichment), JE-008 (entity organisation), JE-009 (viewing), JE-010 (editing/versioning), JE-011 (soft delete), IDX-001..004, RET-*, SRCH-*, DET-*.

## Milestone 1 — current implementation (`internal/entries`, `internal/worker`, `internal/provider`, `internal/server`)

The milestone gate is `internal/worker/integration_test.go`: `TestMilestone1EndToEndEntryToReady` (fake provider; raw file, front matter, canonical row, entry and chunk FTS, section and fact chunks, entities, embeddings, provenance, rule occurrences). All rows asserted there are produced by the pipeline, not fixtures. The Ollama adapter is tested against a stub HTTP server only (`internal/provider/ollama_test.go`).

| Requirement | Test(s) | Status |
|---|---|---|
| JE-001..005 (form, raw Markdown, location, front matter, restrictions) | `internal/entries/entries_test.go`: `TestSaveValidationLimits`, `TestSaveDedupesAndLowercasesTags`, `TestSaveWritesExactlyOneRowAndFile`; `internal/server/server_test.go`: `TestHomeServesFormWithVendoredHTMX`, `TestSaveEntryWritesMarkdownAndCanonicalRows`, `TestInvalidInputRejected`; gate: `TestMilestone1EndToEndEntryToReady` | Complete |
| JE-006 (canonical indexed representation) | `TestSaveEntryWritesMarkdownAndCanonicalRows`; gate test | Complete |
| JE-007 (summary, internal tags, provenance) | `internal/worker/worker_test.go`: `TestProcessingStoresSummaryAndInternalTagsWithProvenanceNotInRawFile`, `TestBlankSummaryFailsTheStepInsteadOfReady` | Complete |
| JE-008 (entities and relations) | `internal/worker/worker_test.go`: `TestEntitiesNormalisedDedupedRejectUnknownAndIdempotentOnReprocess`; gate test | Complete |
| JE-009, JE-010 (view, edit, single previous version, requeue) | `internal/server/server_test.go`: `TestViewShowsSectionsAndEditRedirectsToUpdatedEntry`; `internal/entries/edit_test.go`: `TestEditKeepsIdentityKeepsOneVersionAndReprocessesWithoutDuplicates`, `TestEditValidatesLikeCreateAndLeavesEntryUntouched` | Complete |
| JE-012 (AI-independent save, retry, resync) | `internal/entries/entries_test.go`: `TestSaveLeavesNoOrphanFileWhenDBFails`, `TestSaveRenameFailureLeavesNoRowAndNoFile`, `TestSaveCommitFailureRemovesPublishedFile`; `internal/worker/worker_test.go`: `TestFakeProviderEntryReachesReady`, `TestFailuresRetryThreeTimesWithBackoffThenFail`, `TestSweepAndResyncRequeueFailedAndPendingWhenProviderWasDown`, `TestSweepFailureDoesNotStopWorkers`, `TestSaveSucceedsWhenProviderIsDown`; `TestStatusFragmentPollsUntilTerminalAndResyncRequeues` | Complete |
| IDX-001, IDX-002 (indexed representations, semantic chunks) | `internal/worker/chunks_test.go`: `TestSectionChunksFollowHeadingsWithParentContextAndExactTextIndex`, `TestFactChunksFromProviderWithProvenanceAndReplacedOnReprocess`, `TestChunksLinkOnlyEntitiesTheyMention`; `internal/worker/embeddings_test.go`: `TestEveryChunkEmbeddedWithProvenanceAndNearestNeighbourFindsExpectedChunk`, `TestReprocessingReplacesEmbeddingsWithoutDuplicates`; gate test | Complete |
| IDX-004 (model deployment and provenance) | `internal/provider/ollama_test.go`: `TestOllamaCompleteSendsModelPromptAndOptions`, `TestOllamaEmbedReturnsVector`, `TestOllamaErrorsAreReportedAndTimeoutApplies`; `internal/config/config_test.go`; provenance asserted in gate test | Complete (endpoint, models, timeout, context, params configurable) |
| DET-001..003 (rules, occurrences, evaluation) | `internal/worker/rules_test.go`: `TestMigrationSeedsDetectionRules`, `TestMatchingEntryGetsOccurrencesWithExcerptConfidenceAndProvenance`, `TestNonMatchingEntryHasNoOccurrences`, `TestOccurrencesBelowMinimumConfidenceAreIgnored`, `TestReprocessingNeverDoubleCountsAndDistinctEventsAreSeparate`, `TestRuleVersionChangeRequeuesActiveEntriesAndReplacesOccurrences`, `TestDisabledRuleIsNotEvaluated`; gate test | Complete |
| DATA-001, DATA-002 | gate test (raw file is untouched by processing and holds no generated data); `TestProcessingStoresSummaryAndInternalTagsWithProvenanceNotInRawFile` | Complete |

## Test run log

- `go build ./...` — clean.
- `gofmt -l .` — no files need formatting.
- `go vet ./...` — clean.
- `go test ./...` — all tests pass (`internal/journal`: 19 test functions; `internal/httpapi`: 3 test functions, 7 cases incl. subtests).
- Manual smoke test: built binary, ran against a real temp journal root, `curl`'d `POST /api/entries` for both a happy path and a validation failure, inspected the resulting `.md` file on disk directly.
