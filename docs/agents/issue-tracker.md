# Issue tracker: GitHub Issues

GitHub Issues in `HenriqueMichelini/careeros` are the canonical source of truth for implementation work. The Markdown ticket files formerly stored under `.scratch/` were migrated to Issues #4–#13 on 2026-10-06 and are no longer maintained.

## Canonical workflow

- Search before creating: `gh issue list --repo HenriqueMichelini/careeros --state all`.
- Read the full ticket and its comments: `gh issue view <number> --repo HenriqueMichelini/careeros --comments`.
- Create one Issue per implementation ticket with `gh issue create --repo HenriqueMichelini/careeros --title ... --body-file ...`.
- Update progress, decisions, evidence, and blockers in the Issue with `gh issue comment` or `gh issue edit`.
- Close only when the Issue's acceptance criteria are satisfied; use `gh issue close --reason completed`. Use `--reason "not planned"` only for deliberately abandoned work.
- Keep one feature or workstream per title prefix, such as `[backend-v1]` or `[profile-ingestion]`, and preserve dependency order in the ticket title/body.
- Keep blockers, acceptance criteria, verification evidence, privacy constraints, and domain terminology in the Issue body. Append discussion as comments rather than rewriting history.

## Implementation boundary

GitHub Issues coordinate the work; source code still changes in a local branch. After implementation and verification, push the branch and use `gh pr create` or `gh pr edit` so the remote repository records the change through a reviewable pull request. Do not edit the default branch's files through the GitHub web UI or Contents/API writes.

## Migrated tickets

- Issues #4–#7: `backend-v1-release` verification tickets; closed as completed.
- Issues #8–#11: `backend-v1` implementation tickets 01–04; closed as completed/accepted.
- Issue #12: `backend-v1` Ticket 05; open because the overall v1 release gates remain explicit work.
- Issue #13: `profile-ingestion` Ticket 01; open and ready for implementation after the accepted Ticket 05 baseline.

Do not recreate these tickets under `.scratch/`. New tickets must be GitHub Issues.
