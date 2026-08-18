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

`public_api_contract_digests.json` additionally freezes the complete extracted
surface, including Python package exports and method return annotations and
canonical Go receiver names. Inspect proposed changes with `--print-digests` and
update a digest only after confirming that the change is additive and intended.

The source baseline is one layer of the contract. The shared
`protocol_contract_v1.json` corpus runs against deterministic Python, Go, and
Rust SDK consumers and a dependency-free module frozen at the 0.3.2
compatibility floor. It protects framing, handshake, schema, execution,
unknown-method errors, ignored additive request fields, and shutdown behavior.

The probe sources are compatibility fixtures, not examples: do not update their
SDK calls merely to accommodate a refactor. Extend them additively when a new
capability becomes part of the intended 1.0 contract. Existing SDK tests retain
the deeper coverage for sessions, steps, mesh, credentials, payloads, malformed
frames, and validation behavior.
