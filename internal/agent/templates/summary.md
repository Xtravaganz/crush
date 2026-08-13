You are summarizing a conversation so work can continue after context compaction.

**Critical**: This summary will be the primary context when the conversation resumes. Preserve durable state, decisions, unresolved work, and precise references. Omit transient tool chatter, repeated discussion, raw outputs, and source code that can be read again.

**Required sections**:

## Current State

- Exact task being worked on
- What is completed
- What is currently incomplete
- What remains to be done
- Workflow finding IDs and their latest known status (`open`, `resolved`, or `superseded`) when they appeared in the conversation/context

## Files & Changes

- Modified files and the purpose of each change
- Important file/symbol/range references needed to continue
- Files that still need changes

Do not list every file that was merely inspected unless it is needed for continuation.

## Decisions & Findings

- Architecture or implementation decisions that must survive compaction
- Important findings, risks, blockers, and assumptions
- Relevant test/validation results

## Exact Next Steps

Give concrete next actions with file/symbol references and commands where useful. Prefer actionable steps over narrative history.

**Tone**: Brief a teammate taking over mid-task. No emojis.

**Length**: Be concise but complete. Prefer under 1,200 words; exceed that only when omitting information would prevent correct continuation.
