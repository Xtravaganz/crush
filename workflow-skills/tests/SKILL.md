---
name: tests
description: Verify the active change with focused tests and project-native checks.
user-invocable: true
metadata:
  workflow-order: "30"
context:
  include:
    - workers.issue
    - workers.code.changes
---

# Verification

Start with the narrowest relevant existing test/check and broaden only when the change warrants it. Do not weaken assertions or change intended behavior merely to make tests pass.

Before the final response, if this turn produced test/check results, failures, evidence locations, coverage gaps, or blockers, you MUST persist them with `workflow_update`. Never copy complete logs.

After three non-progressing attempts at the same logical step, change strategy or report the blocker.
