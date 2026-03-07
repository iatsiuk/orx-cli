# Add Reasoning Effort Selection to init TUI

## Overview
- Add a second TUI screen to `orx init` for selecting reasoning effort level per model
- After model selection (Enter), if any selected models support `"reasoning"` in `supported_parameters`, show a screen where user can set effort (none/minimal/low/medium/high/xhigh) per model
- User can skip individual models (no effort set) or skip the entire screen (Enter without changes)
- Selected effort generates `"reasoning": {"effort": "..."}` block in the config file
- Models without effort selection keep `reasoning` in `// available:` comment as before
- Preserve existing user-configured parameters (temperature, reasoning, etc.) when re-running `orx init` - previously all custom params were silently dropped

## Context (from discovery)
- Files/components involved:
  - `internal/modelsel/ui.go` - TUI components, will need second screen
  - `internal/modelsel/modelsel.go` - orchestrator, `Run()` returns `[]config.SelectedModel`
  - `internal/modelsel/api.go` - `APIModel` struct with `SupportedParameters`
  - `internal/config/config.go` - `SelectedModel` struct, `ReasoningConfig`
  - `internal/config/generate.go` - `GenerateFromModels()`, `buildModel()`, `writeModel()`
  - `internal/config/generate_test.go` - generation tests
  - `internal/modelsel/modelsel_test.go` - modelsel tests
  - `cmd/orx/main.go` - `runInitInteractive()`, `mergeDisabledModels()`, `extractPreSelected()`
  - `cmd/orx/main_test.go` - init and merge tests
- Related patterns:
  - TUI uses `tview` library with `tcell` for input handling
  - Tests use stdlib `testing`, `httptest`, table-driven patterns, `t.Parallel()`
  - No testify - only stdlib assertions
- Valid effort values: `none`, `minimal`, `low`, `medium`, `high`, `xhigh` (from `validateReasoning`)
- `SupportedParameters` from OpenRouter API includes `"reasoning"` for reasoning-capable models

## Development Approach
- **Testing approach**: TDD (tests first)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task** - no exceptions
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- TUI logic tests: test reasoning effort data flow without running actual TUI (same pattern as existing `newTuiApp` tests in `modelsel_test.go`)
- Config generation tests: validate generated output by parsing as JSON5 + calling `Config.Validate()`, not just substring checks (avoids flaky assertions from map iteration order)
- Table-driven tests for effort cycling logic
- Integration of reasoning screen into `Run()` is not unit-testable (tview direct construction); cover via pure-function tests (`applyEfforts`, `filterReasoningSelectedModels`) and manual verification

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix
- Update plan if implementation deviates from original scope

## Implementation Steps

### Task 1: Preserve existing config parameters on re-init
Currently `mergeDisabledModels()` creates new `SelectedModel{ID, Name, Enabled}` for disabled models, silently dropping all user-configured parameters (temperature, reasoning, top_p, etc.). `SelectedModel` also lacks fields to carry these values. This task adds an `ExistingParams` field and populates it from the existing config so `GenerateFromModels()` can emit them back.

- [x] write test in `cmd/orx/main_test.go`: `TestMergeDisabledModels_PreservesExistingParams` - disabled model with `Temperature: 0.7` in existing config should have that value in merged result's `ExistingParams`
- [x] write test: disabled model with `Reasoning: {Effort: "high"}` in existing config should preserve reasoning in `ExistingParams`
- [x] write test: enabled (selected) model should also carry `ExistingParams` from existing config if it was previously configured
- [x] write test: model not in existing config should have nil `ExistingParams`
- [x] write test in `internal/config/generate_test.go`: `TestGenerateFromModels_WithExistingParams` - model with `ExistingParams` containing temperature should emit `"temperature": 0.7` in output
- [x] write test: `ExistingParams` should NOT override `DefaultParameters` from API for enabled models - API defaults take precedence
- [x] run tests - expect failures
- [x] add `ExistingParams *Model` field to `SelectedModel` in `internal/config/config.go` (pointer to existing `Model` struct)
- [x] update `mergeDisabledModels()` in `cmd/orx/main.go`: look up existing model by ID and set `ExistingParams` for both disabled and selected models
- [x] update `buildModel()` in `internal/config/generate.go`: for enabled models, merge `ExistingParams` fields as active params (lower priority than API `DefaultParameters`); for disabled models, emit all `ExistingParams` fields
- [x] run tests - must pass before next task

### Task 2: Add ReasoningEffort field to SelectedModel
- [x] write test in `internal/config/generate_test.go`: `TestGenerateFromModels_WithReasoningEffort` - model with `ReasoningEffort: "high"` generates `"reasoning": {"effort": "high"}` in output; validate by parsing output as JSON5 and checking `cfg.Models[0].Reasoning.Effort == "high"`
- [x] write test: model with `ReasoningEffort: "none"` generates active reasoning config (distinct from skip)
- [x] write test: model with empty `ReasoningEffort` keeps `reasoning` in `// available:` comment (existing behavior preserved)
- [x] write test: model with `ReasoningEffort` should NOT have `reasoning` in `// available:` comment
- [x] write test: model that does NOT support `"reasoning"` in `SupportedParameters` but has `ReasoningEffort` set - reasoning should be ignored
- [x] run tests - expect failures (field doesn't exist yet)
- [x] add `ReasoningEffort string` field to `SelectedModel` in `internal/config/config.go`
- [x] update `buildModel()` in `internal/config/generate.go`: when iterating `SupportedParameters`, if param is `"reasoning"` and `m.ReasoningEffort != ""`, add to `ActiveParams` as `config.ReasoningConfig{Effort: m.ReasoningEffort}` and `continue` to skip adding to `AvailableKeys`
- [x] run tests - must pass before next task

### Task 3: Add reasoning effort cycling logic (pure functions)
- [x] write test in `internal/modelsel/modelsel_test.go`: `TestNextEffort` - table-driven, cycles through `"" -> "none" -> "minimal" -> "low" -> "medium" -> "high" -> "xhigh" -> ""`
- [x] write test: `TestSupportsReasoning` - returns true when `SupportedParameters` contains `"reasoning"`, false otherwise
- [x] write test: `TestFilterReasoningSelectedModels` - filters `[]config.SelectedModel` to only those with `"reasoning"` in `SupportedParameters`
- [x] write test: empty `SupportedParameters` returns empty list
- [x] run tests - expect failures
- [x] implement `nextEffort(current string) string` in `internal/modelsel/reasoning.go`
- [x] implement `supportsReasoning(params []string) bool` in `internal/modelsel/reasoning.go`
- [x] implement `filterReasoningSelectedModels(models []config.SelectedModel) []config.SelectedModel` in `internal/modelsel/reasoning.go`
- [x] run tests - must pass before next task

### Task 4: Build reasoning effort TUI screen
- [ ] write test in `internal/modelsel/modelsel_test.go`: `TestReasoningTui_InitialState` - models without existing effort start with `""` (skip)
- [ ] write test: `TestReasoningTui_InitialStateWithExisting` - models with `ExistingParams.Reasoning.Effort` start with that value pre-loaded
- [ ] write test: `TestReasoningTui_CycleEffort` - calling `cycleEffort()` on current item changes effort value
- [ ] write test: `TestReasoningTui_GetEfforts` - returns `map[string]string` of model ID to effort, excludes empty (skipped) entries
- [ ] run tests - expect failures
- [ ] create `reasoningTuiApp` struct in `internal/modelsel/reasoning.go` with fields: `app *tview.Application`, `models []config.SelectedModel`, `efforts map[string]string`, `confirmed bool`
- [ ] implement `newReasoningTuiApp(models []config.SelectedModel) *reasoningTuiApp` - builds TUI with model list showing effort per model; pre-load effort from `model.ExistingParams.Reasoning.Effort` if available
- [ ] implement layout: single list with model IDs and current effort label, status bar with controls; follow `buildComponents()`/`buildLayout()`/`setupInputHandlers()` pattern from `ui.go`
- [ ] implement Space key: cycle effort for current model via `nextEffort()`; extract input handler into separate method to stay within cyclop complexity limit
- [ ] implement Enter key: confirm and stop app
- [ ] implement Esc key: cancel (set confirmed=false, stop) - skips reasoning setup, does not cancel entire init
- [ ] implement `getEfforts() map[string]string` - returns non-empty efforts
- [ ] run tests - must pass before next task

### Task 5: Integrate reasoning screen into modelsel.Run()
- [ ] update `modelsel.go` `Run()`: after model selection TUI, call `filterReasoningSelectedModels()` on selected models
- [ ] if reasoning models exist, create and run `reasoningTuiApp` with filtered models
- [ ] apply returned efforts to corresponding `SelectedModel.ReasoningEffort` before returning
- [ ] if no reasoning models or user cancels reasoning screen (Esc), proceed as before (no effort set)
- [ ] write test: `TestApplyEfforts` - pure function that applies `map[string]string` efforts to `[]SelectedModel`, verify correct models get effort values
- [ ] run tests - must pass before next task

### Task 6: Verify acceptance criteria
- [ ] verify: selecting a reasoning model and setting effort generates `"reasoning": {"effort": "..."}` in config
- [ ] verify: skipping effort for a reasoning model keeps `reasoning` in `// available:` comment
- [ ] verify: non-reasoning models are unaffected
- [ ] verify: second screen is skipped when no reasoning models selected
- [ ] verify: Esc on reasoning screen cancels only reasoning setup, not entire init
- [ ] verify: re-running `orx init` with existing config preserves all user-configured parameters (temperature, reasoning, etc.)
- [ ] verify: existing reasoning effort is pre-loaded on the reasoning TUI screen
- [ ] verify: disabled models in generated config retain their existing parameters
- [ ] run full test suite: `go test ./...`
- [ ] run linter: `make build`
- [ ] verify test coverage for new code

### Task 7: [Final] Update documentation
- [ ] update README.md if init command documentation needs changes

## Technical Details

### Effort cycle order
```
"" (skip) -> "none" -> "minimal" -> "low" -> "medium" -> "high" -> "xhigh" -> "" (skip)
```

### Reasoning TUI screen layout
```
+--[ Reasoning Effort ]-----------------------+
| anthropic/claude-4-opus      effort: [high]  |
| deepseek/deepseek-r1         effort: (skip)  |
| openai/o3                    effort: [medium] |
+----------------------------------------------+
 Space: cycle effort  Enter: done  Esc: skip all
```

### ExistingParams merge priority (for enabled models)
```
1. ReasoningEffort from TUI (highest priority - user just selected it)
2. DefaultParameters from API (provider defaults)
3. ExistingParams from previous config (user's manual edits, lowest priority)
```
For disabled models, only ExistingParams are emitted (no API defaults, no TUI input).

### Generated config output with effort
```json5
{
  "models": [
    {
      "name": "Claude 4 Opus",
      "model": "anthropic/claude-4-opus",
      "enabled": true,
      "reasoning": {"effort": "high"}
      // available: max_tokens, temperature
    }
  ]
}
```

### Data flow
```
runInitInteractive()
  -> config.Load(path) -> existing []config.Model (best-effort)
  -> extractPreSelected(existing) -> []string (model IDs)
  -> modelsel.Run(ctx, token, opts)
       -> tuiApp (model selection) -> []SelectedModel
       -> filterReasoningSelectedModels(selected) -> []SelectedModel (reasoning only)
       -> reasoningTuiApp (effort selection, pre-loads existing efforts) -> map[string]string
       -> applyEfforts(selected, efforts) -> updates SelectedModel.ReasoningEffort
       -> return []SelectedModel
  -> mergeDisabledModels(existing, selected)
       -> selected models: attach ExistingParams from existing config
       -> disabled models: create SelectedModel with ExistingParams preserved
       -> return []SelectedModel (all models)
  -> GenerateFromModels(all)
       -> buildModel(): merge ExistingParams + DefaultParameters + ReasoningEffort
       -> writeModel(): output JSON with all active params
```

## Post-Completion

**Manual verification:**
- Run `orx init` with a real API token
- Select a mix of reasoning and non-reasoning models
- Verify second screen appears only for reasoning models
- Verify generated config has correct `reasoning` blocks
- Test cancel flows (Esc on both screens)
- Edit generated config manually (add temperature, reasoning effort, etc.)
- Re-run `orx init` - verify existing parameters are preserved in output
- Verify reasoning TUI pre-loads existing effort values from config
