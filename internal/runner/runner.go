package runner

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"orx/internal/client"
	"orx/internal/config"
)

type savedOutput struct {
	Name    string `json:"name"`
	Model   string `json:"model"`
	Content string `json:"content"`
}

func saveOutput(dir string, result *client.Result) (string, error) {
	id := generateUUID()

	data, err := json.MarshalIndent(savedOutput{
		Name:    result.Name,
		Model:   result.Model,
		Content: result.Content,
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal output: %w", err)
	}

	path := filepath.Join(dir, "orx-"+id+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("write output: %w", err)
	}

	return path, nil
}

func generateUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

const defaultTimeout = 600 * time.Second

type Runner struct {
	client      *client.Client
	models      []config.Model
	timeout     time.Duration
	progressOut io.Writer
	saveDir     string
	progressMu  sync.Mutex
}

type Option func(*Runner)

func WithTimeout(d time.Duration) Option {
	return func(r *Runner) { r.timeout = d }
}

func WithProgressOut(w io.Writer) Option {
	return func(r *Runner) { r.progressOut = w }
}

func New(models []config.Model, cl *client.Client, saveDir string, opts ...Option) *Runner {
	r := &Runner{
		client:  cl,
		models:  models,
		saveDir: saveDir,
		timeout: defaultTimeout,
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

type Output struct {
	Results         []client.Result `json:"results"`
	TotalDurationMs int64           `json:"total_duration_ms"`
	TotalCost       float64         `json:"total_cost"`
	Successful      int             `json:"successful"`
	Failed          int             `json:"failed"`
}

func (r *Runner) Run(ctx context.Context, systemPrompt, prompt string) (*Output, error) {
	if len(r.models) == 0 {
		return nil, config.ErrNoEnabledModels
	}

	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	start := time.Now()
	results := make([]client.Result, len(r.models))
	var mu sync.Mutex

	g, gctx := errgroup.WithContext(ctx)

	for i := range r.models {
		g.Go(func() error {
			r.progress(r.models[i].Name, "requesting")

			result := r.client.Execute(gctx, &r.models[i], systemPrompt, prompt)

			mu.Lock()
			results[i] = result
			mu.Unlock()

			if result.Status == "success" {
				doneMsg := fmt.Sprintf("done (%.1fs)", float64(result.DurationMs)/1000)
				r.trySaveOutput(r.models[i].Name, &result, doneMsg)
			} else {
				r.progress(r.models[i].Name, "error")
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

func (r *Runner) trySaveOutput(name string, result *client.Result, doneMsg string) {
	path, err := saveOutput(r.saveDir, result)
	if err != nil {
		r.progress(name, doneMsg)
		r.progress(name, fmt.Sprintf("save error: %v", err))
		return
	}
	r.progress(name, fmt.Sprintf("%s, %s", doneMsg, path))
}

func (r *Runner) progress(name, status string) {
	if r.progressOut == nil {
		return
	}
	r.progressMu.Lock()
	_, _ = fmt.Fprintf(r.progressOut, "%s - [%s]\n", name, status)
	r.progressMu.Unlock()
}
