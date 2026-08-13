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

Act read-only. Compare requirements with diff and verification evidence. Focus on correctness, regressions, security, types, concurrency, compatibility, and material test gaps. Ignore style.

## Analysis Dimensions

Perform **all** for every new feature or significant change:

1. **Data-flow**: Trace UI → API → worker → storage. Every hop validates, authorizes, bounds data.
2. **Security**: Authz at every endpoint, permission scoping, no plaintext secrets (logs/tasks/browser), rate-limiting.
3. **Concurrency**: Shared mutable state → atomic ops or locking. TOCTOU, duplicate processing, last-write-wins.
4. **Deployment**: Dockerfiles ship runtime assets, paths resolve from container CWD, env vars read. Rolling-deploy safe.
5. **Migration**: Existing data valid after schema/algorithm changes. Migration or accept-and-upgrade path.
6. **Error paths**: Every async op caught, streams have error handlers, blocking ops timed out. No orphaned state.
7. **Tests**: Flag zero-coverage on critical paths — security, error handling, concurrency.
8. **Consistency**: Related files (models, DTOs, UI, workers, Dockerfiles, configs) updated together. No dead code, stale refs, mismatched contracts.

## Depth

Trace actual execution paths — surface-level pattern matching is not enough. Find what an adversarial reviewer would catch.

Persist findings, risks, missing verification, side effects, verdict with `workflow_update` before responding. Review is read-only toward repo files.

After three non-progressing attempts at the same step, change strategy or report the blocker.
