---
name: hovel-sessions
description: Discover, inspect, read from, and deliberately interact with established Hovel sessions without launching new throws.
license: Apache-2.0
compatibility: Requires Hovel 0.4.x and a configured Hovel MCP server.
metadata:
  hovel-min-version: "0.4.0"
  hovel-max-version: "0.5.0"
---

# Hovel sessions

List existing sessions through the typed session capability, select the session
that matches the user's operation and target, then inspect
`hovel_session_capabilities`. Read current context before writing. Invoke an
advertised operation with `hovel_session_call` only when required by the request,
then read the resulting state.

Do not create a throw to satisfy a request about an already established session.
Do not assume every session is a shell or invent commands absent from its
capabilities. Keep payload installation, throw execution, and session interaction
as distinct lifecycles.
