---
name: remember
description: Save session learnings to memory, retrieve past memories, or curate the memory store. Usage: /remember [what to save or search for]
---

# Memory Management

Use the `memory` tool directly to manage project memories.

## Save (default when user provides content)
User says: `/remember the auth middleware was rewritten for compliance`

Call `memory` with action `save` and the content to persist. Keep memories concise (under 500 characters), factual, and self-contained. Use a markdown heading as the first line.

For "remember this session" — summarize the key outcomes, decisions, and discoveries from the current conversation and save each distinct topic as a separate memory.

## Retrieve
User says: `/remember what do we know about providers?`

Call `memory` with action `list`, then filter the results to answer the user's question.

## Curate
User says: `/remember clean up stale entries`

Call `memory` with action `list`, review for duplicates or stale entries, and present findings. Use `memory delete` to remove entries the user confirms should go.

## Guidelines
- Each memory should have a clear `# Title` as the first line
- Keep each under 500 characters — memories compete for a shared budget in the system prompt
- Separate concerns: prefer 2 focused memories over 1 sprawling one
- Do not save obvious facts derivable from code, standard patterns, or things already in KODA.md
- Do not delete memories without presenting what would be removed first
