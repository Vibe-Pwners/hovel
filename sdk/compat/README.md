# Module compatibility contract

The checked-in `public_api_baseline.json` records the currently shipped public
Python, Go, and Rust SDK source surface. `//sdk/compat:public_api_baseline_test`
treats that surface as additive-only: removing a symbol or changing its recorded
signature fails the SDK gate. Additions are allowed while Hovel works toward the
1.0.0 contract.

Refresh the baseline only for an intentional, compatibility-reviewed expansion:

```text
aspect run //sdk/compat:public_api_baseline_test -- --print
```

The source baseline is one layer of the contract. Existing SDK and module-example
tests also exercise lifecycle calls, request/response encoding, sessions, mesh,
credentials, and packaging. Before declaring the interface 1.0.0, add shared
language-neutral wire fixtures and execute them against all three SDKs; add
old-module/new-runtime black-box fixtures; and publish explicit rules for default
values, unknown fields, error payloads, and capability negotiation. Those areas
are behavioral contracts and cannot be proven by source signatures alone.
