package modelsel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

var ErrNoTextModels = errors.New("no text models available")

type SelectedModel struct {
	ID                  string
	Name                string
	SupportedParameters []string
	DefaultParameters   map[string]any
}

type Options struct {
	Verbose  bool
	VerboseW io.Writer
	BaseURL  string
}

// Run displays TUI for model selection. Returns nil if user cancelled.
func Run(ctx context.Context, token string, opts *Options) ([]SelectedModel, error) {
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

	app := newTuiApp(sorted)
	if err := app.run(); err != nil {
		return nil, fmt.Errorf("run TUI: %w", err)
	}

	if !app.confirmed {
		return nil, nil
	}

	return app.getSelectedModels(), nil
}
