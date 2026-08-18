# Hovel safety model

- A throw requires a persisted plan and a recorded confirmation.
- Confirmation is bound to the exact plan hash. Any plan change requires review
  and confirmation of the new plan.
- `--now` or its typed equivalent bypasses a prompt, not auditing or policy.
- Modules tagged `dangerous` require explicit dangerous-module authorization.
- Launch-key or peer approval remains required when policy requests it.
- Never hide, redact, invent, or silently discard operator configuration.
- Report blockers and required approvals; do not work around them through a
  generic command escape hatch.
