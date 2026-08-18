# Hovel workflow

1. Inspect operator identity, workspace state, and available capabilities.
2. Select or create an operation and bind intended targets.
3. Inspect candidate modules and build or update a chain.
4. Supply required configuration and validate the chain.
5. If execution is requested, create and review a persisted throw plan.
6. Confirm the exact plan hash and satisfy launch-key policy.
7. Start only the confirmed plan, then observe status, logs, sessions, and artifacts.

Refresh the workspace snapshot after mutations instead of assuming local state is
current. Prefer capability discovery over guessing provider-specific operations.
