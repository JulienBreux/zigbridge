# AI Telemetry & Smart Recommendations

Zigbridge includes an on-demand **Smart Recommendation Engine** designed to discover device usage patterns and recommend optimal direct hardware bindings and scenes.

---

## Philosophy: On-Demand & Human-Gated

Many smart home users don't know which switches can be directly bound to which lights, or they forget that direct binding is possible.

Zigbridge solves this without intrusive background automation:
- **No Background Guessing**: The engine runs strictly **on-demand** when requested via the Web Dashboard or API.
- **Human-in-the-Loop**: Recommendations are presented with a confidence score and plain-English rationale; no binding is written to hardware flash until you click **Apply**.
- **Zero Cloud Requirement**: The default heuristic engine is 100% offline and deterministic.

---

## 1. Circular Telemetry Ring Buffer

Zigbridge maintains a thread-safe, bounded circular buffer in memory (`max_event_history: 2000`). It records recent network interactions:
- Device reports and command invocations (e.g., button press timestamps).
- State changes (e.g., light bulb on/off transitions).
- Discovered device clusters and endpoints.

Because the buffer is bounded, memory usage remains strictly capped at a few kilobytes regardless of how long Zigbridge runs.

---

## 2. Recommendation Engines

You can configure the analyzer engine in `config.yaml`:

```yaml
ai:
  enabled: true
  engine: rule_based         # Options: "rule_based" or "external_llm"
  min_confidence: 0.75       # Filter proposals below this confidence (0.0 to 1.0)
  max_event_history: 2000
```

### Option A: Deterministic Rule-Based Engine (`rule_based`)
The rule-based analyzer evaluates network topology and cluster capabilities:
- Matches devices with output clusters (e.g., a switch with `0x0006` On/Off or `0x0008` Level Control) with devices containing matching input clusters (e.g., a dimmable light bulb).
- Correlates temporal event patterns (e.g., switch button press followed within 500ms by an automation turning on a specific bulb).
- Generates instant direct binding proposals without requiring external APIs or GPU resources.

### Option B: External LLM Hook (`external_llm`)
For advanced topology reasoning, Zigbridge can dispatch an anonymized snapshot of your network topology and event logs to a local LLM (e.g. Ollama running Llama 3 or Mistral) or an external OpenAI-compatible endpoint:

```yaml
ai:
  engine: external_llm
  llm_endpoint: "http://localhost:11434/v1/chat/completions"
  llm_api_key: ""
```

The LLM receives:
- List of device types and cluster capabilities.
- Anonymized event interaction sequence.

It responds with structured JSON proposals explaining *why* a particular direct binding or multi-way switch binding will reduce latency and improve reliability.

---

## 3. Reviewing & Applying Recommendations

In the Web Dashboard:
1. Navigate to the **Smart Suggestions** tab.
2. Click **Analyze Network**.
3. Inspect the proposed bindings, confidence score, and rationale (e.g., *"Living Room Dimmer Remote has matching Level Control cluster (0x0008) with Ceiling Light. Direct binding will eliminate 180ms latency"*).
4. Click **Apply Direct Binding** to commit the binding to hardware flash.
