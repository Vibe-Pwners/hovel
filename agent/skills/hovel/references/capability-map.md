# Capability map

Use typed discovery tools to obtain current details from the running Hovel:

- identity and collaborators: `hovel_operator_identity`, operator entity tools
- state: `hovel_workspace_snapshot`, operation and chain capability tools
- modules: `hovel_catalog_snapshot`, `hovel_module_search`, `hovel_module_inspect`
- construction: `hovel_chain_suggest`, `hovel_chain_apply`, validation tools
- execution: `hovel_launch_key_policy`, `hovel_throw_plan`,
  `hovel_throw_confirm`, `hovel_throw_start`
- payloads: `hovel_installed_payload_list`, `hovel_payload_capabilities`,
  `hovel_payload_call`
- sessions: session list tools, `hovel_session_capabilities`, `hovel_session_call`
- artifacts: typed artifact list and inspection capability tools

This map is strategic, not a schema reference. Read the MCP tool descriptions and
input schemas exposed by the connected Hovel version before calling a tool.
