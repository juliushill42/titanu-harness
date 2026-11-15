# TitanU LittleBit Edge Backend

A clean-room, Go-native execution backend for an individual LittleBit-style factorized linear layer. It is additive to TitanU Harness and builds for Linux and Android ARM64/Termux without Python, CUDA, or external runtime dependencies.

## Research and licensing boundary

Samsung's LittleBit paper describes factorized binary matrices with row, column, and latent scales, plus optional residual compensation. LittleBit-2 changes initialization using latent rotation and Joint-ITQ; the reference README states that this adds no inference-time overhead. Samsung's reference repository is CC BY-NC 4.0. TitanU does not copy or vendor that implementation. This backend independently implements the published inference equation and accepts TitanU's own artifact schema.

This is a layer backend, not a complete Transformer implementation and not a claim that Samsung checkpoints load directly. A checkpoint exporter must map each trained layer into this schema, and a Transformer/tokenizer runtime must compose layers before end-to-end text generation is possible.

## Artifact format: titanu-littlebit-layer-v1

JSON fields: schema_version=1, input_dim, output_dim, rank, primary, and optional residual. Dimensions and rank must be positive. Each path contains:
- u_sign: standard Base64 of row-major packed bits for output_dim × rank
- v_sign: standard Base64 of row-major packed bits for input_dim × rank
- h: output_dim float32 scales
- g: input_dim float32 scales
- l: rank float32 scales

Bits are least-significant-bit first. A set bit represents +1; a clear bit represents -1. The layer computes:

    y = diag(h) U_sign diag(l) V_sign^T diag(g) x

An optional residual path is added to the primary output. The implementation never expands a dense weight matrix.

## Commands

The repository bootstrap builds bin/titanu-littlebit after running all Go tests and vet checks.

    bin/titanu-littlebit inspect -artifact layer.json
    bin/titanu-littlebit apply -artifact layer.json -input 0.25,-0.5

## Evidence boundaries

Tests cover sign convention, factorized output, residual addition, malformed schemas, non-finite inputs, and storage metadata. packed_parameter_bytes reports packed sign bytes plus float32 scale bytes in this schema; it excludes JSON/Base64 serialization and allocator/runtime overhead. dense_fp16_parameter_bytes is a dense FP16 matrix comparison, not a measured whole-model reduction.

The implementation is CPU-native scalar Go, not architecture-specific SIMD, CUDA, or Android NNAPI. No speedup, battery improvement, perplexity, or whole-device memory claim is made until measured on physical devices with a complete model.
