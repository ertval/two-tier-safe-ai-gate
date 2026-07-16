# Execution Plan: Two-Tier Safe AI Gate

This document outlines the design decisions and implementation plan for the `two-tier-safe-ai-gate` reference architecture.

---

## 🎯 Objectives
- **System Reliability:** Restrict LLM agents from directly executing state-changing API operations.
- **Architectural Separation:**
  - **Tier 1 (Autonomous Exploration):** Agent interprets human instructions and plans actions in a safe sandbox.
  - **Deterministic Policy Boundary:** Independent verification gates validate action parameters (e.g. transfer limits, risk scores) before dispatch.
  - **Tier 2 (Durable Execution):** A fault-tolerant, stateful step-coordinator ensures eventual consistency (automatic backoff, retries, and timeouts).
- **Go Ecosystem Alignment:** Leverage Go 1.25's type safety and lightweight execution model.

---

## 🛠️ Step-by-Step Implementation Map

### Step 1: Core Schemas and Policy Engine
- Define `Transaction` schema (amount, currency, risk score, user ID).
- Code `VerifyPolicy` boundary in Go:
  - Check transaction limits (payouts > $500 require human intervention).
  - Evaluate risk scores (risk > 0.8 automatically aborted).

### Step 2: Tier 1 Agent Loop & Mock Fallback
- Build `agent/loop.go` using `github.com/sashabaranov/go-openai` to structure raw user requests.
- Provide a robust regex/substring-matching mock fallback if no `OPENAI_API_KEY` is present.

### Step 3: Tier 2 Bounded Durable Execution
- Set up **Inngest** Go SDK handlers in `workflow/durable.go`.
- Map the execution steps:
  1. Policy Gate Verification.
  2. Suspend/Wait for event (`agent/payout.approved`) for high-value transactions.
  3. Execute Payment step (mock network call with retry).
  4. Write immutable audit log step.

### Step 4: Model Context Protocol (MCP) Server Integration
- Implement a standard JSON-RPC 2.0 stdio loop.
- Expose the deterministic `verify_transaction_policy` tool to any external Omnigent runtime.

### Step 5: Composition & Deployment
- Write `main.go` supporting dual running mode (FastAPI REST endpoints or Stdio MCP server).
- Compose a `docker-compose.yml` orchestrating the Go app and the Inngest Dev Server container.
