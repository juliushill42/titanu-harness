# TitanU Harness — Working Local Control Plane

This repository contains the **interactive Harness interface wired to a real Go backend**. The 2026-09-25 build record described a separate Rust/Go system; **that original Rust source, PostgreSQL/Kafka sinks and packaged archives were not present** when this work started. This code implements a self-contained, local-only runtime without misrepresenting those legacy components.

## Start (Linux or Termux)

```bash
bash bootstrap.sh
```

The script verifies tests and vet results, builds the local runtime, and prints a 64-character access token. Open **http://127.0.0.1:8844** on the **same device**, enter the token, and use the live panels. The access token also lives in `data/control.token` as 32 raw bytes (shown in hex on launch). Never commit `data/` or share the token.

To run checks without launching: `bash bootstrap.sh test`.

## What the buttons actually do

- **Policy check** — evaluates operation + path through Go policy authority and an independent Go mirror. Disagreement rejects.
- **Execute** — performs real `write`, `read`, `hash` or `list` actions under `data/workspace/`. Shell commands are **always refused**. Writes are capped at 128 KiB, traversal and symlink escapes are blocked on Linux.
- **Proof** — appends HMAC-SHA256 signed, SHA-256 chained JSONL events (`data/ledger.jsonl`) and verifies on-disk evidence before subsequent mutations.
- **Replay** — compares a recorded file hash to current workspace state; it never reruns an action.
- **Learning** — exports actual recorded execution events to SFT JSONL, and human-submitted preferred/rejected responses to preference JSONL. Empty sources yield empty exports.
- **Routing** — submits prompts to an explicitly configured loopback-only, OpenAI-compatible local model endpoint. It reports `MODEL_NOT_CONFIGURED` without a model; it does not simulate one.
- **Historic evidence** — the original design's September 25 historical evidence remains labeled separately from fresh runtime data.

## Local model integration (optional)

Start a compatible model server bound to loopback, then set its actual endpoint before starting Harness:

```bash
TITANU_MODEL_ENDPOINT=http://127.0.0.1:8080/v1/chat/completions TITANU_MODEL_ID=local bash bootstrap.sh
```

No model, weights or cloud services are installed by Harness. Configure only a model process you actually run.

## API (all except health and login require authentication)

`GET /health`, `POST /api/login`, `POST /api/logout`, `GET /api/state`, `POST /api/evaluate`, `POST /api/execute`, `GET /api/events`, `GET /api/verify`, `GET /api/replay/{id}`, `POST /api/feedback`, `GET /api/export/sft`, `GET /api/export/preferences`, `POST /api/infer`.

All writes require `Content-Type: application/json`. Browser requests are constrained to same-origin. CLI clients use `Authorization: Bearer <printed-64-character-token>`. Default binding is `127.0.0.1:8844`; exposing this HTTP server to an untrusted network is **not supported**. For CLI checks:

```bash
TOKEN=$(od -An -tx1 -v data/control.token | tr -d ' \n')
curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8844/api/verify
curl -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"tool":"write","path":"notes/hello.txt","content":"Hello"}' \
  http://127.0.0.1:8844/api/execute
```

## Proof boundaries and limitations

- HMAC key and ledger share a local filesystem; this detects changes against the held key, **not** remote or hardware-rooted attestation. Back up keys and ledgers together using safe local handling.
- No unsandboxed shell, external tool execution, actual Rust binary, Kubernetes, PostgreSQL, Kafka, remote access, cross-host concurrency or automatic model download is claimed.
- File writes are atomic via temporary file + rename but ledger append and file update are not a single atomic transaction. Interrupted writes can be reflected as a missing completion event. Do not use this service as the only source of high-assurance audit evidence.
- Only one server instance should open a given `data` directory. Existing state is retained across restart.
- Runtime proofs in tests use a temporary directory and real local HTTP, including traversal/symlink denial, altered-ledger detection and model-route integration to a local test server. These tests do not establish production availability.

## Layout

- `index.html`, `live.css`, `live.js`: interactive frontend
- `cmd/harness`: server entrypoint
- `internal/harness`: policy, workspace tools, signed ledger, exports, HTTP API, inference route and tests
- `bootstrap.sh`: one-command test, build and launch
- `.github/workflows/ci.yml`: CI checks
- `data/`: local secrets, audit and workspace, ignored by Git

## Executed verification record

`proof/local-verification.json` records the real local Go race tests, vet, JavaScript syntax check, Android ARM64 cross-build, and clean-install HTTP proof (health, policy, ledger). It is dated test evidence, not a guarantee about a different host.

## Version

Implementation: local runtime version **0.2.0**. Historical record: September 25, 2026 version 0.1.0, cited separately.
