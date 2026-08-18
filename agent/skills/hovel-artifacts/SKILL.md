---
name: hovel-artifacts
description: Locate, inspect, and explain Hovel result artifacts while preserving their operation, chain, target, and audit context.
license: Apache-2.0
compatibility: Requires Hovel 0.3.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.3.0"
  hovel-max-version: "0.4.0"
---

# Hovel artifacts

Refresh workspace state, use typed artifact listing and inspection capabilities,
and filter by the operation, chain, target, module, or throw named by the user.
Preserve provenance and distinguish an artifact from the payload that produced it
and any session it describes.

Do not execute a new throw merely to retrieve an existing result. Do not modify,
redact, or discard operator-controlled values when reporting artifact metadata.
If content is unavailable, report the artifact identity and the exact access
failure rather than guessing.
