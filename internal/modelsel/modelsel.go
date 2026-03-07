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
	Verbose     bool
	VerboseW    io.Writer
	BaseURL     string
	PreSelected []string
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

	reasoningModels := filterReasoningSelectedModels(selected)
	if len(reasoningModels) > 0 {
		rApp := newReasoningTuiApp(reasoningModels)
		if err := rApp.run(); err != nil {
			return nil, fmt.Errorf("run reasoning TUI: %w", err)
		}
		if rApp.confirmed {
			selected = applyEfforts(selected, rApp.getEfforts())
		}
	}

	return selected, nil
}
