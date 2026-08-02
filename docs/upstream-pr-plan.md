# Upstream PR plan

This fork carries 29 commits ahead of `upstream/main` (icereed/paperless-gpt),
25 of them real app changes (the other 4 are fork-internal CI/build chores —
see "Excluded from upstream" below). Before opening PRs, each commit was
checked for real dependencies (shared types/functions, not just textual diff
proximity) so they can be split into the smallest reviewable, independently
mergeable units.

`upstream/main` has moved since this plan was first written (it now includes
upstream's own OCR Playground & Activity feature, PR #1005, and its
follow-up polish, PR #1018) — but neither touches the same files as any row
below, so the dependency order and "applies cleanly today" status are
unaffected. Re-verify with `git merge-base --is-ancestor <upstream-pr-commit>
upstream/main` before assuming that holds indefinitely.

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

## New commits (2026-07-30 → 2026-08-02)

Landed after this plan was first written. Checked against the same method.

| # | Commit | Depends on | Notes |
|---|---|---|---|
| 16 | `fix: use absolute entrypoint path and skip root-only setup when non-root` | none | Docker/ops fix only (entrypoint.sh, Dockerfile); applies cleanly today |
| 17 | `fix: grant correspondents and document types the same owner/permission plumbing as tags` | #5 | refactors `createdTagRequestPayload` from #5 into a shared `objectOwnerAndPermissionFields` helper, extends it to Correspondent/DocumentType |
| 18 | `feat: show OCR run status in the document picker` | none (upstream #1005) | reads/writes only new code (`ocr_runs.go`, `types.go`, `DocumentPicker.tsx`); depends only on OCR Playground types already in `upstream/main` via upstream PR #1005, not on any unmerged row here |
| 19 | `fix: require children for auto-detected tag parent candidates` | #15 | fixes a bug in `buildDetailedTagsWithParentCandidates`/`hasExplicitParentCandidates`, both introduced by #15 |
| 20 | `feat: add zoomable scan preview to review and OCR views` | none (upstream #1005) | new shared `ImageZoomModal` component wired into `FocusReview.tsx`/`RunResults.tsx`, both introduced by upstream PR #1005 already in `upstream/main` — applies cleanly today, no fork-only dependency |

## Excluded from upstream

Fork-internal CI/build-infrastructure commits — never upstream PR candidates,
kept only for this fork's own GHCR publishing pipeline:

- `chore(ci): retrigger build after linking GHCR package Actions access`
- `chore(ci): retrigger build with corrected GHCR write permissions`
- `fix(ci): lowercase GHCR repo name in Docker build workflow`
- `docs: record upstream PR split and dependency order` (this file's own
  first commit)

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

Rows 16, 18, and 20 apply cleanly against `upstream/main` today and can be
opened alongside rows 1–9 — none of them wait on any other row in this plan.
Row 17 only needs row 5 merged first; row 19 only needs row 15 (which needs
row 12) merged first.
