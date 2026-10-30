# Technical debt

This folder records known design or implementation problems that we know of. Each one is written up in enough detail that someone new to that part of the code can understand it, reproduce it and plan a fix.

## Index

| Document | Area | Severity | Summary |
|---|---|---|---|
| [Daemon dataplane: missing teardown and non-idempotent state](daemon-dataplane-teardown-and-idempotency.md) | `daemon/` | High | Per-app and per-target dataplane state is never torn down and is keyed by name. Allocators never release, and ServiceChain routes are append-only. Re-applying deleted manifests breaks service chains, pods stop starting after about 13 restarts per app per node, and unlocked maps can crash the daemon. Related: after an operator restart, its CIDR allocator can hand P4Targets CIDRs that overlap node pod CIDRs (D1). |

## Adding an entry

1. Create `<area>-<short-topic>.md` in this folder.
2. Start with a header table: area, severity, status, and the commit the line numbers refer to.
3. Explain:
   - **what** is wrong, with repo-relative links to the code;
   - **how to observe it**, with concrete commands or logs;
   - the **impact**;
   - a **suggested approach**.
4. Add a row to the index above.
5. When the debt is paid off, update the status in the document (or delete it) and its row here, in the same PR as the fix.
