# TitanU Harness — Control Plane Interface

Interactive visual control-plane interface for TitanU Harness.

## Open it

Open `index.html` in a modern browser. No build step, dependencies, account, cloud connection, or API key is required.

## Interface

- Cinematic central Harness visualization and interactive module selection
- Policy engine overview
- Proof/evidence inspection
- Execution and behavioral replay views
- Learning/export and inference-routing views
- Command and execution overview
- Evidence search and filtering
- Responsive desktop and mobile layouts

## Evidence and data integrity

The interface displays **historical, example/build-record content** from the TitanU Harness build record dated September 25, 2026. It is **not** streaming live runtime status, calling Harness APIs, or independently verifying tests. User interaction changes the view and filters existing content only.

## Repository scope

This repository currently contains the **standalone control-plane interface**, not the Rust policy engine, Go orchestrator, execution sandbox, proof engine, inference router, training exporters, or their source/tests.

## Integration boundary

Production integration requires binding the visual components to authenticated backend APIs and feeding the UI with signed verification records from the actual Harness service. Until then, treat the interface as a working frontend, not a backend or proof-generation system.
