# Issue tracker: GitHub

Issues and specs live in GitHub Issues for `wbe7/hermes-operator`. Use the `gh` CLI; infer the repository from the git remote or pass `--repo wbe7/hermes-operator`.

## Conventions

- Create an issue: `gh issue create --title "..." --body-file <file>`.
- Read an issue: `gh issue view <number> --comments`; fetch labels when needed.
- List issues: `gh issue list --state open --json number,title,body,labels,comments`, with appropriate label and state filters.
- Comment: `gh issue comment <number> --body-file <file>`.
- Apply or remove labels: `gh issue edit <number> --add-label "..." --remove-label "..."`.
- Close: `gh issue close <number>`; write any explanatory comment separately using `--body-file`.

Write multiline bodies to a file with real newlines. Preserve the exact text passed for publication. An instruction to publish to the issue tracker means creating a GitHub issue; fetching a relevant ticket means reading the issue and its comments.

## Pull requests as a triage surface

**PRs as a request surface: no.**

If this flag is changed to `yes`, triage external PRs using the same label roles and the corresponding `gh pr` commands. Read both `gh pr view <number> --comments` and `gh pr diff <number>`. External request authors have `authorAssociation` of `CONTRIBUTOR`, `FIRST_TIME_CONTRIBUTOR`, or `NONE`; exclude `OWNER`, `MEMBER`, and `COLLABORATOR`.

Issues and PRs share a number space. When a number is ambiguous, try `gh pr view` and fall back to `gh issue view`.

## Wayfinding operations

Used when invoking the wayfinder workflow:

- Map: one issue labelled `wayfinder:map`, containing Notes, Decisions-so-far, and Fog.
- Child ticket: a GitHub sub-issue of the map with `wayfinder:<type>` (`research`, `prototype`, `grilling`, or `task`). If sub-issues are unavailable, use a task list in the map and `Part of #<map>` in the child.
- Blocking: use native GitHub issue dependencies. Add a dependency through `gh api --method POST repos/wbe7/hermes-operator/issues/<child>/dependencies/blocked_by -F issue_id=<blocker-db-id>`. The blocker ID is the issue database ID from `gh api repos/wbe7/hermes-operator/issues/<number> --jq .id`, not its display number or node ID. If dependencies are unavailable, put `Blocked by: #<number>` references in the child body.
- Frontier: among the map's open children, exclude assigned tickets and those with open blockers; select the first remaining ticket in map order. For native dependencies, read `issue_dependencies_summary.blocked_by`.
- Claim: assign the chosen ticket to the driving developer with `gh issue edit <number> --add-assignee @me`.
- Resolve: comment with the result using `--body-file`, close the child, then append a concise result and link to the map's Decisions-so-far section.
