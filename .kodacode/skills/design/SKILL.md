---
name: design
description: Structure your analysis as 2-3 design options with tradeoffs, a recommendation, and a decision gate before committing to an approach
suggests:
  before: [brainstorm]
---

# Design Exploration

When this skill is loaded, structure your response using the following format. Do not skip sections.

## 1. Constraints

List the hard requirements and non-negotiables for this task. Include:
- What MUST be true (functional requirements, compatibility, performance bounds)
- What MUST NOT happen (breaking changes, data loss, security regressions)
- Scope boundaries (what is explicitly out of scope)

## 2. Options

Present 2-3 distinct approaches. For each option:

### Option N: <Name>
- **Summary**: 1-2 sentences describing the approach
- **How it works**: Key implementation steps (specific files, functions, patterns)
- **Gains**: What you get — be concrete (faster by X, simpler API, fewer files to change)
- **Costs**: What you pay — be concrete (new dependency, migration needed, breaks X consumers)
- **Risk**: What could go wrong

Do not pad with a third option if only two are genuinely distinct.

## 3. Recommendation

State which option you recommend and why in 2-3 sentences. Reference the specific tradeoffs that make it the best fit for the constraints above.

## 4. Decision

Ask the user to choose:
- Which option to proceed with
- Whether to explore any option further before deciding
- Whether the constraints are missing something

Do NOT proceed to implementation until the user has made a choice.
