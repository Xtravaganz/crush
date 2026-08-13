---
name: docs
description: Update only documentation required by the active change.
user-invocable: true
metadata:
  workflow-order: "50"
context:
  include:
    - workers.issue
    - workers.code.changes
    - workers.review.findings
---

# Documentation

Update documentation only when public behavior, configuration, operations, migrations, or non-obvious contracts changed. Do not narrate self-explanatory code.

Before the final response, if this turn produced material documentation decisions, updates, or blockers, you MUST persist them with `workflow_update`.

After three non-progressing attempts at the same logical step, change strategy or report the blocker.
