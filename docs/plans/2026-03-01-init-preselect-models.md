# Pre-select existing config models in init TUI

## Overview
When `orx init` runs and a config file already exists, pre-select previously configured models in the TUI. After user confirms selection:
- Selected models -> `enabled: true`
- Deselected models (were in old config, unchecked in TUI) -> `enabled: false`
- Stale models (were in old config, no longer in API) -> `enabled: false`

## Context
- `internal/config/config.go` -- `Config`, `Model`, `SelectedModel`, `Load()`
- `internal/config/generate.go` -- `GenerateFromModels()`, `buildModel()`, `WriteConfig()`
- `internal/modelsel/modelsel.go` -- `Run()`, `Options`
- `internal/modelsel/ui.go` -- `tuiApp`, `newTuiApp()`, `getSelectedModels()`
- `cmd/orx/main.go` -- `runInitInteractive()`
- Tests: `internal/config/generate_test.go`, `internal/modelsel/modelsel_test.go`

## Development Approach
- **Testing approach**: TDD (tests first)
- Complete each task fully before moving to the next
- Make small, focused changes
- **CRITICAL: every task MUST include new/updated tests** for code changes in that task
- **CRITICAL: all tests must pass before starting next task**
- **CRITICAL: update this plan file when scope changes during implementation**
- Run tests after each change with `make build`
- Maintain backward compatibility

## Testing Strategy
- **Unit tests**: required for every task
- Test both success and error scenarios
- No external mocking libraries (stdlib `testing` only)
- `go test -race ./...` for race detection

## Progress Tracking
- Mark completed items with `[x]` immediately when done
- Add newly discovered tasks with + prefix
- Document issues/blockers with ! prefix

## Implementation Steps

### Task 1: Add `Enabled` field to `SelectedModel` and update `GenerateFromModels`
- [x] write test in `generate_test.go`: `TestGenerateFromModels_DisabledModel` -- model with `Enabled: false` generates `"enabled": false` in output, no params
- [x] write test in `generate_test.go`: `TestGenerateFromModels_MixedEnabledDisabled` -- mix of enabled/disabled models generates correct `enabled` values
- [x] add `Enabled bool` field to `SelectedModel` in `config.go`
- [x] update `buildModel()` in `generate.go` to use `m.Enabled` instead of hardcoded `true`
- [x] update `getSelectedModels()` in `ui.go` to set `Enabled: true`
- [x] update existing tests in `generate_test.go` to set `Enabled: true` where needed (fix `TestGenerateFromModels_MultipleModels` and others)
- [x] run `make build` -- must pass before next task

### Task 2: Add `PreSelected` to `modelsel.Options` and `newTuiApp`
- [x] write test in `modelsel_test.go`: `TestPreSelectModels` -- verify `newTuiApp` with preSelected IDs populates `selected` map correctly (only for models that exist in list)
- [x] write test in `modelsel_test.go`: `TestPreSelectModels_NonExistent` -- preSelected ID not in model list is ignored
- [x] add `PreSelected []string` field to `Options` struct in `modelsel.go`
- [x] change `newTuiApp(models []APIModel)` signature to `newTuiApp(models []APIModel, preSelected []string)`
- [x] implement pre-selection: build available set from models, populate `selected` map for matching IDs
- [x] update `Run()` to pass `opts.PreSelected` to `newTuiApp()`
- [x] run `make build` -- must pass before next task

### Task 3: Load existing config and merge disabled models in `runInitInteractive`
- [x] write test in `main_test.go`: `TestMergeDisabledModels` -- helper function that computes disabled models from old config vs selected result
- [x] write test: old config model deselected in TUI -> appears in result with `Enabled: false`
- [x] write test: old config model not in API -> appears in result with `Enabled: false`
- [x] write test: old config model selected in TUI -> not duplicated, stays `Enabled: true`
- [x] write test: no existing config -> no disabled models
- [x] extract `mergeDisabledModels(existing []config.Model, selected []config.SelectedModel) []config.SelectedModel` function
- [x] update `runInitInteractive()`: try `config.Load(path)` before TUI, extract enabled model IDs for `PreSelected`
- [x] update `runInitInteractive()`: after TUI, call `mergeDisabledModels()` to append disabled entries
- [x] run `make build` -- must pass before next task

### Task 4: Verify acceptance criteria
- [ ] verify: existing config with enabled models -> pre-selected in TUI
- [ ] verify: existing config with disabled models -> NOT pre-selected in TUI
- [ ] verify: deselecting a model -> `enabled: false` in new config
- [ ] verify: model not in API -> `enabled: false` in new config
- [ ] verify: no existing config -> clean start, no pre-selection
- [ ] verify: config load error (corrupted file) -> graceful fallback, no pre-selection
- [ ] run full test suite with `go test -race ./...`
- [ ] run `make build` -- linter must pass

## Technical Details

### Data flow
```
runInitInteractive()
  |-- config.Load(path) -> existing models (may fail, that's OK)
  |-- extract preSelected IDs (only enabled: true models)
  |-- modelsel.Run(ctx, token, &Options{PreSelected: preSelected})
  |     |-- fetchModels -> API models
  |     |-- filterTextModels
  |     |-- sortByProvider
  |     |-- newTuiApp(sorted, preSelected) -> pre-populate selected map
  |     |-- user interacts with TUI
  |     |-- return selected models (Enabled: true)
  |-- mergeDisabledModels(existingModels, selected)
  |     |-- for each old config model not in selected -> SelectedModel{Enabled: false}
  |-- GenerateFromModels(selected + disabled)
  |-- WriteConfig()
```

### `mergeDisabledModels` logic
```
input: existing []config.Model, selected []config.SelectedModel
output: []config.SelectedModel (full list: selected enabled + missing disabled)

1. build selectedSet from selected model IDs
2. for each model in existing:
     if model.Model not in selectedSet:
       append SelectedModel{ID: model.Model, Name: model.Name, Enabled: false}
3. return append(selected, disabled...)
```

## Known Limitations (future iterations)

- **Invalid config breaks pre-selection**: `config.Load()` calls `Validate()`, so a manually
  edited config with invalid values (e.g., `temperature: 5`) causes Load to fail and
  pre-selection falls back to empty. All old models must be re-selected manually.
  Fix: add `LoadRaw()` that parses JSON5 without validation, extracting model IDs/names
  for pre-selection while ignoring invalid parameters.

## Post-Completion

**Manual verification:**
- Run `orx init` with existing config, verify pre-selection works in TUI
- Run `orx init` without existing config, verify clean start
- Verify deselecting models produces `enabled: false` entries
- Delete a model from OpenRouter (simulate by editing config with non-existent model ID), re-run init
