---
name: sync
description: Explicitly publish a concise workflow result to the linked GitLab/GitHub issue or merge request.
user-invocable: true
disable-model-invocation: true
context:
  shared: true
  include:
    - workers.code.changes
    - workers.tests.results
    - workers.review
---

# Explicit remote sync

Never sync automatically. Only publish when the user explicitly invokes this skill. Use existing authenticated tooling and publish a concise human-readable summary, not the workflow YAML. Never expose credentials.
