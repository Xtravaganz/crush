---
name: context
description: Inspect or repair the active workflow YAML when explicitly requested.
user-invocable: true
disable-model-invocation: true
context:
  shared: true
  include:
    - workers
  writable:
    - shared
    - workers
---

# Workflow context maintenance

Use only when explicitly requested to inspect, repair, or consolidate the active workflow state. Preserve unknown fields. Keep durable facts and file/symbol/range references only; remove accidental source dumps and raw tool logs.
