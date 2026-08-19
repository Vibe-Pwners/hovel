---
name: hovel-payloads
description: Inspect installed Hovel payloads and invoke advertised provider-neutral payload capabilities safely.
license: Apache-2.0
compatibility: Requires Hovel 0.4.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.4.0"
  hovel-max-version: "0.5.0"
---

# Hovel payloads

Use `hovel_installed_payload_list` before selecting a payload. Inspect the
selected payload with `hovel_payload_capabilities`, read the advertised schema,
then call the provider-neutral operation with `hovel_payload_call`.

Do not infer operations from a payload name or provider. Do not confuse an
installed payload with a target, artifact, or established session. Prefer the
capability-driven tools over `hovel_payload_cmd`, command-specific compatibility
tools, or `hovel_command_run` when the typed path is available.

After a mutation, inspect the returned state and any resulting artifact or
session before making another call.
