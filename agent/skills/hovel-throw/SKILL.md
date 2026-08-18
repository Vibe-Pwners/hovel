---
name: hovel-throw
description: Safely plan, review, confirm, start, and observe a Hovel throw when the user explicitly requests execution.
license: Apache-2.0
compatibility: Requires Hovel 0.3.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.3.0"
  hovel-max-version: "0.4.0"
---

# Hovel throw

Execution is a separate, explicit workflow. Never turn successful validation
directly into `hovel_throw_start`.

1. Refresh workspace state and validate the selected operation and chain.
2. Inspect `hovel_launch_key_policy`.
3. Call `hovel_throw_plan` to persist the exact plan.
4. Review its targets, modules, configuration, risk, dangerous tags, approvals,
   and plan hash with the user.
5. Only after explicit execution intent, call `hovel_throw_confirm` for that exact
   hash, including dangerous authorization or prompt bypass only when requested.
6. Satisfy peer or launch-key approval. Do not impersonate another operator.
7. Call `hovel_throw_start` only for the unchanged confirmed plan.
8. Observe typed status and logs; report sessions and artifacts produced.

Never recreate or alter a plan between confirmation and start unless the old
approval is intentionally invalidated. A `now` bypass must still be recorded and
does not bypass dangerous-module or launch-key policy.
