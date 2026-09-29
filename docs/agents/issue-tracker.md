# Issue tracker: Local Markdown

Issues for this repo live in `.scratch/`. Backend v1 uses `.scratch/backend-v1/issues/`, with `docs/backend-requirements.md` as its agreed specification. Do not create a second, divergent specification for that effort.

## Conventions

- Keep one feature per directory: `.scratch/<feature-slug>/`.
- Keep one implementation ticket per file at `.scratch/<feature-slug>/issues/<NN>-<slug>.md`, numbered from `01` in dependency order.
- Record each ticket's prerequisites on a `Blocked by:` line and its triage role on a `Status:` line near the top. The initial published status is `ready-for-agent`.
- Append discussion to a ticket under `## Comments` rather than overwriting its history.

## Skill operations

- To publish a ticket, create its numbered Markdown file. Do not combine multiple tickets into one file.
- To fetch a ticket, read the referenced file and its blockers. If only a number is given, find that number within the named feature directory.
- To find the implementation frontier, start with the lowest-numbered ticket whose blockers are complete. For backend v1, the approved dependencies form a linear sequence.
