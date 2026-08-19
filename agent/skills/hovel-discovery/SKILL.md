---
name: hovel-discovery
description: Discover Hovel workspace state, operations, chains, modules, collaborators, payloads, sessions, and artifacts before changing anything.
license: Apache-2.0
compatibility: Requires Hovel 0.4.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.4.0"
  hovel-max-version: "0.5.0"
---

# Hovel discovery

Begin with `hovel_operator_identity`, then `hovel_workspace_snapshot` and
`hovel_catalog_snapshot`. Establish the active operation and chain, intended
targets, other connected operators, installed modules, payloads, sessions, and
artifacts relevant to the request.

Do not create an operation, chain, target, payload, or throw when an appropriate
one already exists. Use `hovel_module_search` to narrow candidates and
`hovel_module_inspect` before recommending one. Treat the running Hovel catalog
and advertised capability schemas as authoritative.

Return a concise state summary and identify missing prerequisites. Discovery is
read-only; do not start a throw or interact with a session merely because one is
present.
