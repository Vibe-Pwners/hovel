---
name: hovel-troubleshooting
description: Diagnose Hovel MCP, workspace, catalog, validation, approval, payload, session, and artifact failures without bypassing safety policy.
license: Apache-2.0
compatibility: Requires Hovel 0.4.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.4.0"
  hovel-max-version: "0.5.0"
---

# Hovel troubleshooting

Preserve the exact error, then refresh `hovel_operator_identity`,
`hovel_workspace_snapshot`, and `hovel_catalog_snapshot` as applicable. Determine
whether the cause is missing context, stale state, invalid configuration,
unavailable capability, disconnected provider, or unmet approval policy.

Correct one prerequisite at a time and revalidate. If a throw plan changes,
review and confirm its new hash. Never retry destructive tools blindly, bypass a
dangerous-module or launch-key requirement, or fall back to `hovel_command_run`
solely to evade a typed-tool error.

Report what changed, what remains blocked, and the next safe operator action.
