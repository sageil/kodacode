# Agent Coordination Design Notes

## Context

Claude Relay demonstrates a useful pattern: multiple local Claude Code sessions can exchange natural-language messages through a per-session MCP channel and a local hub daemon. That is a good lightweight transport, but it is not enough as the long-term coordination model for Kodacode.

The stronger direction is to treat cross-session communication as durable agent coordination infrastructure, with natural language as the interface and structured events as the system contract.

## Why Typed Coordination Is Better Than Session Chat

Session chat solves the demo problem: one agent can ask another agent a question. Typed coordination solves the engineering problem: multiple agents can reliably divide work, report state, recover from failure, and leave an audit trail.

Key advantages:

- It survives restarts. Pending asks, replies, task ownership, and workflow evidence should not disappear if a hub or session restarts.
- It is debuggable. The system should be able to answer who asked what, which session received it, whether it replied, whether it timed out, and which workflow decision used the result.
- It avoids protocol ambiguity. Natural language can remain the user interface, but the internal request should be structured: request type, target, topic, timeout, required artifact, context references, and correlation IDs.
- It fits workflows. Most agent coordination is not open-ended chat; it is task assignment, blocking questions, review requests, evidence sharing, handoff, and completion reporting.
- It handles offline or busy sessions. A durable coordinator can queue work, expire requests, retry, or route to another capable peer.
- It creates a source of truth. Ownership, active tasks, current reviews, superseded answers, and workflow evidence should live in one queryable state model.
- It can scale past one machine. A local daemon can be the first implementation, but the protocol should support remote peers, auth, and access control later.

## Preferred Shape

Build a coordination service that owns:

- session registry and heartbeats
- peer capabilities and roles
- inboxes and outboxes
- durable message/event storage
- request acknowledgements and timeouts
- task ownership and handoff state
- workflow evidence produced by peer replies
- audit and replay

For local-first operation, this can be a daemon backed by SQLite and a Unix socket. For multi-machine use, the same protocol can move to WebSocket, SSE, or gRPC with authentication.

## Protocol Direction

Use natural language to let users say:

```text
ask backend if the auth schema changed
```

Translate that into a structured coordination request:

```json
{
  "type": "question",
  "to": "backend",
  "topic": "auth schema",
  "requires_reply": true,
  "timeout_seconds": 300,
  "context_refs": ["file://server/auth/schema.ts"]
}
```

Agents can still read and write human-friendly text, but routing, retries, UI state, workflow advancement, and auditing should depend on the structured record.

## Event Model Candidates

Potential event types:

- `peer_registered`
- `peer_heartbeat`
- `peer_status_changed`
- `message_sent`
- `message_delivered`
- `message_read`
- `message_replied`
- `message_failed`
- `task_claimed`
- `task_released`
- `handoff_requested`
- `handoff_completed`
- `review_requested`
- `review_completed`
- `workflow_evidence_recorded`

## Kodacode Fit

This should integrate with Kodacode's existing session, task, workflow, and event model rather than existing as a transient side channel.

Likely first-class tools:

- `peer_status`
- `peer_ask`
- `peer_reply`
- `peer_broadcast`
- `peer_claim_task`
- `peer_handoff`
- `peer_review_request`

Likely TUI surfaces:

- peer/session status panel
- inbox of pending coordination requests
- visible ownership for active tasks
- workflow evidence rows for peer replies
- timeout and retry state for unanswered asks

## Design Principle

Claude Relay is a clever transport. Kodacode should aim for durable, typed coordination between agents, with natural language as the front end and event-sourced workflow state as the source of truth.
