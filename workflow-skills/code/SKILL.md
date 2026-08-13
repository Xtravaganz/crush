---
name: code
description: Implement the smallest correct change for the active task.
user-invocable: true
metadata:
  workflow-order: "20"
context:
  include:
    - workers.issue
    - workers.review.findings
---

# Implementation

Implement the smallest coherent change that satisfies the active task. Inspect before editing, preserve unrelated work, and avoid unrequested dependency or API changes.

Before the final response, if this turn produced durable hypotheses, working-set references, material changes, side effects, blockers, or next steps, you MUST persist them with `workflow_update`. Never store full source or raw tool output.

After three non-progressing attempts at the same logical step, change strategy or report the blocker.
