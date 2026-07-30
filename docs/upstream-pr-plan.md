# Upstream PR plan

This fork carries 20 commits ahead of `upstream/main` (icereed/paperless-gpt).
Before opening PRs, each commit was checked for real dependencies (shared
types/functions, not just textual diff proximity) so they can be split into
the smallest reviewable, independently mergeable units.

## Dependency-checked order

Commits with "none" in the Depends-on column apply cleanly against
`upstream/main` today and can be opened as PRs in any order, right now.

| # | Commit | Depends on | Notes |
|---|---|---|---|
| 1 | `fix: throttle LLM retry attempts` | none | smallest, safest first PR |
| 2 | `fix: align tag hierarchy with paperless v3` | none | |
| 3 | `fix: keep OCR saves out of tag conflicts` | none | |
| 4 | `fix: flatten nested Paperless tags` | none | |
| 5 | `fix: configure created tag matching and permissions` | none | |
| 6 | `Handle Mistral OCR image references` | none | |
| 7 | `feat: review tag changes as deltas` | none | backend-only |
| 8 | `feat: review custom fields individually` | none | |
| 9 | `feat: add taxonomy context to suggestions` | none | |
| 10 | `feat: add title schema and document type review` | none | foundational — unblocks #11, #12 |
| 11 | `feat: combine core metadata suggestions into a single LLM call` | #10 | uses `settings.TitleSchema`, `DocumentTypeOption`; unblocks #13, #14 |
| 12 | `feat: add hierarchical tag review` | #10 | uses `DocumentTypeOption`; unblocks #15 |
| 13 | `fix: allow compound document types` | #11 | edits `metadata_prompt.tmpl`, only exists after #11 |
| 14 | `feat: support suggested tag removals` | #11 | uses `coreMetadataSuggestion` / `getSuggestedCoreMetadata` |
| 15 | `feat: support parent-aware tag suggestions` | #12 | edits `tag_hierarchy.go`, only exists after #12 |
| — | `fix: clean ocr scratch and prune jobs` | none checked | OCR hardening cluster, likely independent — send after building review trust |
| — | `fix: harden ocr jobs and mistral retry` | none checked | same cluster |
| — | `fix: preserve mistral OCR table content` | none checked | same cluster |
| — | `fix: preserve partial suggestion results` | none checked | same cluster |
| — | `fix: clean up frontend lint errors` | none checked | trivial, bundle with whichever frontend PR goes first |

## Verification method

Real dependencies were confirmed by tracing symbol/type definitions across
commits (e.g. `grep` for `DocumentTypeOption`, `tag_hierarchy.go`,
`coreMetadataSuggestion`) rather than relying on raw `git cherry-pick`
conflicts, which also fire on incidental same-file line proximity and would
have over-counted dependencies.

## Next step

Open PRs for rows 1–9 first (independent, no ordering constraints). Prioritize
row 10 (`title schema and document type review`) once a couple of the small
ones have landed, since two larger features are gated behind it.
