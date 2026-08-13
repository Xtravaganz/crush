---
name: issue
description: Analyze the active task and maintain scope, acceptance criteria, plan, risks, and open questions.
user-invocable: true
metadata:
  workflow-order: "10"
---

# Issue planning

Understand the active issue or local task. Inspect only enough repository context to define scope and acceptance criteria.

Do not implement the solution in this worker. Before the final response, if this turn produced durable planning facts, decisions, risks, blockers, or next steps, you MUST persist them with `workflow_update`. Keep `workers.issue` concise.

After three non-progressing attempts at the same logical step, change strategy or report the blocker.
