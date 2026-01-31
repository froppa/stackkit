# Struktly

**Struktly** is a governed, fail-first, deterministic code generation system.

It operates on the principle that **no code is better than bad code**.

## Core Principles

1. **Fail-First**: If the generated code does not compile, pass tests, or satisfy policy, it is **discarded**. There are no silent retries.
2. **Determinism**: The same inputs (Spec + Policy) always produce the same scaffolding. AI is only used for bounded logic synthesis within strict templates.
3. **Auditability**: Every run produces an audit trail (`audit.json`, `policy.json`, `verification.json`). If verification fails, you get an audit log explaining why, but no code.
4. **No Hallucinated Intent**: If the system is unsure (simulated by "ambiguous" input), it returns `NEEDS_HUMAN_REVIEW` rather than guessing.

## Architecture

- **CLI**: The entry point.
- **Orchestrator**: Manages the pipeline (Input -> Policy -> Generate -> Synthesize -> Verify -> Output).
- **Generator**: Uses `text/template` to create deterministic `main.go` and handler skeletons.
- **Synthesizer**: Fills in handler logic and tests. In a real system, this calls an LLM. Here, it is mocked to demonstrate success/failure modes.
- **Gates**: Runs `go fmt`, `go vet`, and `go test` inside a sandbox (temp directory).
- **Policy Engine**: Enforces constraints (e.g., max handlers).

## Usage

Build the tool:
```bash
go build -o struktly ./cmd/struktly
```

Run a generation:
```bash
./struktly --spec examples/struktly/petstore.yaml --policy examples/struktly/policy.yaml --out output_dir
```

## Failure Scenarios (Demo)

The included `demo_struktly.sh` script demonstrates three scenarios:

1. **Success**: Valid inputs produce a working Go service with passing tests.
2. **Failure**: Inputs that trigger "bad code" generation (simulated) result in **no output code**, only audit logs.
   - *Why?* Because `go vet` or `go test` failed.
3. **Ambiguity**: Inputs that trigger uncertainty result in `NEEDS_HUMAN_REVIEW`.
   - *Why?* The system refuses to guess business logic.

## What is NOT Supported

- **Retries**: If it fails, fix the input or the prompt. Do not loop until it "looks right".
- **AI Architecture**: The AI cannot add new files, change the router, or pick dependencies. It only fills the function bodies it is assigned.
- **Framework Magic**: The generated code is standard library Go.
