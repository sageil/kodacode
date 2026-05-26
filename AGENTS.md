# AGENT.md

This file defines the working rules for agents contributing to `kodacode`.

## Core standard

When choosing between a fast patch and a clean design, prefer the clean design unless the simpler fix is also structurally correct.

## Non-negotiables

1. Do not add shortcuts, fake state, or UI illusions to hide backend problems.
2. Do not patch around architectural confusion when the real issue is unclear ownership or a broken contract.
3. Do not preserve bad abstractions just because they already exist in the old system.

## Engineering priorities

1. Correctness
2. Clear system boundaries
3. Maintainability
4. Cost effectiveness
5. User benefit
6. Speed of implementation
