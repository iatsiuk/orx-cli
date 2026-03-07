package modelsel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"orx/internal/config"
)

var ErrNoTextModels = errors.New("no text models available")

type Options struct {
	Verbose        bool
	VerboseW       io.Writer
	BaseURL        string
	PreSelected    []string
	ExistingModels []config.Model
}

// Run displays TUI for model selection. Returns nil if user cancelled.
func Run(ctx context.Context, token string, opts *Options) ([]config.SelectedModel, error) {
	if opts == nil {
		opts = &Options{}
	}
	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	models, err := fetchModelsWithURL(ctx, token, baseURL, opts.Verbose, opts.VerboseW)
	if err != nil {
		return nil, fmt.Errorf("fetch models: %w", err)
	}

	filtered := filterTextModels(models)
	if len(filtered) == 0 {
		return nil, ErrNoTextModels
	}

	sorted := sortByProvider(filtered)

	app := newTuiApp(sorted, opts.PreSelected)
	if err := app.run(); err != nil {
		return nil, fmt.Errorf("run TUI: %w", err)
	}

	if !app.confirmed {
		return nil, nil
	}

	selected := app.getSelectedModels()
	attachExistingParams(selected, opts.ExistingModels)

	reasoningModels := filterReasoningSelectedModels(selected)
	if len(reasoningModels) > 0 {
		selected, err = runReasoningTUI(selected, reasoningModels)
		if err != nil {
			return nil, err
		}
	}

	return selected, nil
}

func attachExistingParams(selected []config.SelectedModel, existing []config.Model) {
	if len(existing) == 0 {
		return
	}
	byID := make(map[string]*config.Model, len(existing))
	for i := range existing {
		byID[existing[i].Model] = &existing[i]
	}
	for i := range selected {
		if ep, ok := byID[selected[i].ID]; ok {
			selected[i].ExistingParams = ep
		}
	}
}

func runReasoningTUI(selected, reasoningModels []config.SelectedModel) ([]config.SelectedModel, error) {
	rApp := newReasoningTuiApp(reasoningModels)
	if err := rApp.run(); err != nil {
		return nil, fmt.Errorf("run reasoning TUI: %w", err)
	}
	if rApp.confirmed {
		return applyEfforts(selected, rApp.getEfforts()), nil
	}
	return selected, nil
}
