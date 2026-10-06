# Issue state and triage

GitHub issue state is canonical. The old `Status:` lines remain inside migrated Issue bodies as historical context, but they are not a second tracker.

| Work meaning | GitHub representation | Meaning |
| --- | --- | --- |
| Needs evaluation | Open issue; optionally label `needs-triage` | Scope or priority still needs a decision |
| Waiting for information | Open issue; optionally label `needs-info` | Blocked on missing input or an explicit user decision |
| Ready for agent | Open issue; optionally label `ready-for-agent` | Specified and ready to implement once blockers clear |
| Ready for human | Open issue; optionally label `ready-for-human` | Requires a human action or decision |
| Completed | Closed issue with reason `completed` | Acceptance criteria are satisfied |
| Won't fix | Closed issue with reason `not planned` | Deliberately abandoned or rejected |

Use `gh issue list`, `gh issue view`, `gh issue edit`, `gh issue comment`, and `gh issue close` to maintain this state. If labels are available in the repository, apply them consistently; never encode new ticket state by creating Markdown files under `.scratch/`.
