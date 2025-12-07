package runner

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"orx/internal/client"
	"orx/internal/config"
)

type Runner struct {
	client      *client.Client
	config      *config.Config
	timeout     time.Duration
	progressOut io.Writer
}

func New(cfg *config.Config, cl *client.Client, timeout time.Duration, progressOut io.Writer) *Runner {
	return &Runner{
		client:      cl,
		config:      cfg,
		timeout:     timeout,
		progressOut: progressOut,
	}
}

type Output struct {
	Results         []client.Result `json:"results"`
	TotalDurationMs int64           `json:"total_duration_ms"`
	TotalCost       float64         `json:"total_cost"`
	Successful      int             `json:"successful"`
	Failed          int             `json:"failed"`
}

func (r *Runner) Run(ctx context.Context, prompt string) (*Output, error) {
	models := r.config.EnabledModels()
	if len(models) == 0 {
		return nil, config.ErrNoEnabledModels
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	start := time.Now()
	results := make([]client.Result, len(models))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for i := range models {
		g.Go(func() error {
			r.progress(models[i].Name, "requesting")

			result := r.client.Execute(gctx, &models[i], r.config.SystemPrompt, prompt)

			mu.Lock()
			results[i] = result
			mu.Unlock()

			if result.Status == "success" {
				r.progress(models[i].Name, fmt.Sprintf("done (%.1fs)", float64(result.DurationMs)/1000))
			} else {
				r.progress(models[i].Name, "error")
			}

			return nil
		})
	}

	_ = g.Wait()

	output := &Output{
		Results:         results,
		TotalDurationMs: time.Since(start).Milliseconds(),
	}

	for i := range results {
		if results[i].Status == "success" {
			output.Successful++
			if results[i].Cost != nil {
				output.TotalCost += *results[i].Cost
			}
		} else {
			output.Failed++
		}
	}

	return output, nil
}

func (r *Runner) progress(name, status string) {
	if r.progressOut != nil {
		_, _ = fmt.Fprintf(r.progressOut, "%s - [%s]\n", name, status)
	}
}
