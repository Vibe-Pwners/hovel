---
name: hovel-chain-building
description: Build, configure, inspect, and validate Hovel operations and chains for intended targets without starting execution.
license: Apache-2.0
compatibility: Requires Hovel 0.3.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.3.0"
  hovel-max-version: "0.4.0"
---

# Hovel chain building

Inspect `hovel_workspace_snapshot` and `hovel_catalog_snapshot` first. Reuse a
suitable operation or chain when one exists.

For a new workflow:

1. Determine or create the operation and intended targets.
2. Search the module catalog and inspect candidate modules.
3. Use `hovel_chain_suggest` when it helps, then apply the deliberate result with
   `hovel_chain_apply` or the relevant typed capability tools.
4. Fill required configuration without dropping operator-provided values.
5. Validate and resolve every validation error.
6. Refresh the workspace snapshot and report the resulting chain.

Prefer typed tools over `hovel_command_run`. Validation establishes readiness,
not consent. Never call `hovel_throw_start` as part of chain construction.
