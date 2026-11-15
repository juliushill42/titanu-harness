# TitanU Harness — Working Local Control Plane

This repository contains the interactive Harness interface wired to a real Go backend. The original September 25 build record described a separate Rust/Go system; that original Rust source, PostgreSQL/Kafka sinks and packaged archives were not present when this work started. This code implements a self-contained, local-only runtime without misrepresenting those legacy components.

## Start (Linux or Termux)

```bash
bash bootstrap.sh
```

The script verifies tests and vet results, builds the local runtime, and prints a 64-character access token. Open **http://127.0.0.1:8844** on the same device, enter the token, and use the live panels. The token also lives in `data/control.token`; never commit or share it.

Run checks without launching: `bash bootstrap.sh test`.

## What the buttons actually do

- **Policy check** — evaluates operation + path through Go policy authority and an independent Go mirror. Disagreement rejects.
- **Execute** — performs real write, read, hash or list actions under `data/workspace/`. Shell commands are always refused. Writes are capped at 128 KiB; traversal and symlink escapes are blocked on Linux.
- **Proof** — appends HMAC-SHA256 signed, SHA-256 chained JSONL events and verifies on-disk evidence before subsequent mutations.
- **Replay** — compares a recorded file hash to current workspace state; it never reruns an action.
- **Learning** — exports actual recorded execution events to SFT JSONL, and human-submitted preferred/rejected responses to preference JSONL.
- **Routing** — submits prompts to an explicitly configured loopback-only, OpenAI-compatible local model endpoint. It reports MODEL_NOT_CONFIGURED without a model.
- **Historic evidence** — original historical evidence remains labeled separately from fresh runtime data.

## LittleBit edge backend

The new `internal/littlebit` package evaluates factorized binary linear layers using packed sign matrices and row/column/latent scales, with optional residual compensation. It does not materialize a dense weight matrix.

```bash
bin/titanu-littlebit inspect -artifact testdata/littlebit-layer.json
bin/titanu-littlebit apply -artifact testdata/littlebit-layer.json -input 3,5
```

The bootstrap builds this CLI and executes a deterministic runtime proof. See `docs/LITTLEBIT_EDGE.md` for the artifact contract and mathematical definition.

**Research/license boundary:** Samsung's reference LittleBit repository is CC BY-NC 4.0. TitanU does not vendor or copy its implementation. This backend independently implements the published factorized inference equation. It is a layer execution backend, not a full Transformer/tokenizer or direct loader for Samsung training checkpoints. A compatible checkpoint exporter and complete model runtime are still required for text generation.

## Local model integration (optional)

Start a compatible model server bound to loopback, then set its actual endpoint before starting Harness:

```bash
TITANU_MODEL_ENDPOINT=http://127.0.0.1:8080/v1/chat/completions TITANU_MODEL_ID=local bash bootstrap.sh
```

No model, weights or cloud services are installed by Harness. Configure only a model process you actually run.

## API

`GET /health`, `POST /api/login`, `POST /api/logout`, `GET /api/state`, `POST /api/evaluate`, `POST /api/execute`, `GET /api/events`, `GET /api/verify`, `GET /api/replay/{id}`, `POST /api/feedback`, `GET /api/export/sft`, `GET /api/export/preferences`, `POST /api/infer`.

All endpoints except health and login require authentication. Browser requests are same-origin. CLI clients use Authorization: Bearer token. Default binding is 127.0.0.1:8844; exposing this HTTP server to an untrusted network is not supported.

## Proof boundaries and limitations

- HMAC key and ledger share a local filesystem; this detects changes against the held key, not remote or hardware-rooted attestation.
- No unsandboxed shell, external tool execution, actual Rust binary, Kubernetes, PostgreSQL, Kafka, remote access, cross-host concurrency or automatic model download is claimed.
- File writes are atomic via temporary file + rename but ledger append and file update are not one atomic transaction.
- Only one server instance should open a given data directory. Existing state is retained across restart.
- LittleBit tests validate a CPU factorized layer using a deterministic fixture. They do not establish whole-model perplexity, tokens/second, battery, thermal behavior, or physical Android device performance. Storage figures exclude JSON/Base64 serialization and runtime overhead. No GPU or SIMD acceleration is claimed.

## Layout

- `index.html`, `live.css`, `live.js`: interactive frontend
- `cmd/harness`: authenticated server entrypoint
- `cmd/littlebit`: LittleBit artifact inspection and layer execution CLI
- `internal/harness`: policy, workspace tools, signed ledger, exports, HTTP API and tests
- `internal/littlebit`: clean-room factorized layer backend and tests
- `bootstrap.sh`: one-command test, build, proof and launch
- `data/`: local secrets, audit and workspace, ignored by Git
