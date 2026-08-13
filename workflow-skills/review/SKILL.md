---
name: review
description: Review the active change independently for correctness, regressions, risks, and missing verification.
user-invocable: true
metadata:
  workflow-order: "40"
context:
  include:
    - workers.issue
    - workers.code.changes
    - workers.tests.results
    - workers.tests.failures
---

# Review

Act read-only. Compare the requirement with the current diff and verification evidence. Focus on correctness, regressions, security, types, concurrency, compatibility, and material test gaps; ignore style-only preferences.

Before the final response, if this turn produced findings, risks, missing verification, side effects, or a verdict, you MUST persist them with `workflow_update`. Review remains read-only toward repository files.

After three non-progressing attempts at the same logical step, change strategy or report the blocker.
