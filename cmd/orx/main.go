package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"orx/internal/client"
	"orx/internal/config"
	"orx/internal/files"
	"orx/internal/modelsel"
	"orx/internal/runner"
)

var version = "dev"

var (
	ErrTokenRequired = errors.New("API token required: use --token or set OPENROUTER_API_KEY")
	ErrEmptyPrompt   = errors.New("empty prompt")
	errInitConfig    = errors.New("init config error")
)

type options struct {
	configPath   string
	timeout      int
	token        string
	promptFile   string
	verbose      bool
	files        []string
	maxFileSize  string
	maxTokens    int
	systemPrompt string
}

func main() {
	if err := newRootCmd().Execute(); err != nil {
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		if errors.Is(err, errInitConfig) {
			os.Exit(3)
		}
		_, _ = fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(3)
	}
}

func newRootCmd() *cobra.Command {
	opts := &options{
		timeout: 600,
	}

	rootCmd := &cobra.Command{
		Use:               "orx",
		Short:             "OpenRouter eXecutor - parallel LLM queries",
		Version:           version,
		SilenceUsage:      true,
		SilenceErrors:     true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, opts)
		},
	}
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})

	rootCmd.Flags().StringVarP(&opts.configPath, "config", "c", "", "config file (default: ~/.config/orx.json)")
	rootCmd.Flags().IntVarP(&opts.timeout, "timeout", "t", 600, "global timeout in seconds")
	rootCmd.PersistentFlags().StringVar(&opts.token, "token", "", "OpenRouter API key (default: $OPENROUTER_API_KEY)")
	rootCmd.Flags().StringVarP(&opts.promptFile, "prompt-file", "p", "", "read prompt from file")
	rootCmd.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "dump HTTP request/response")
	rootCmd.Flags().StringArrayVarP(&opts.files, "file", "f", nil, "file paths to include (can be repeated)")
	rootCmd.Flags().StringVar(&opts.maxFileSize, "max-file-size", "64KB", "max size per file (e.g., 64KB, 1MB)")
	rootCmd.Flags().IntVar(&opts.maxTokens, "max-tokens", 100000, "max estimated tokens in file content")
	rootCmd.Flags().StringVarP(&opts.systemPrompt, "system", "s", "", "system prompt")

	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Generate configuration file with interactive model selection",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd, opts)
		},
	}
	initCmd.Flags().StringP("output", "o", "", "output path (default: ~/.config/orx.json)")
	initCmd.Flags().Bool("template", false, "generate template config without interactive selection")
	rootCmd.AddCommand(initCmd)

	return rootCmd
}

func run(cmd *cobra.Command, opts *options) error {
	apiToken := getAPIToken(opts.token)
	if apiToken == "" {
		_ = cmd.Usage()
		return ErrTokenRequired
	}

	cfg, err := config.Load(opts.configPath)
	if err != nil {
		_ = cmd.Usage()
		return fmt.Errorf("load config: %w", err)
	}

	prompt, err := readPrompt(os.Stdin, opts.promptFile)
	if err != nil {
		_ = cmd.Usage()
		return fmt.Errorf("read prompt: %w", err)
	}

	prompt, err = appendFileContent(prompt, opts)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	var verboseOut io.Writer
	if opts.verbose {
		verboseOut = os.Stderr
	}
	cl := client.New(apiToken, opts.verbose, verboseOut)
	r := runner.New(cfg.EnabledModels(), cl, os.TempDir(),
		runner.WithTimeout(time.Duration(opts.timeout)*time.Second),
		runner.WithProgressOut(os.Stderr),
	)

	output, err := r.Run(ctx, opts.systemPrompt, prompt)
	if err != nil {
		return err
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}

	return checkExitCode(output)
}

type exitError struct {
	code int
}

func (e *exitError) Error() string {
	return fmt.Sprintf("exit code %d", e.code)
}

func getAPIToken(flagToken string) string {
	if flagToken != "" {
		return flagToken
	}
	return os.Getenv("OPENROUTER_API_KEY")
}

func checkExitCode(output *runner.Output) error {
	if output.Failed == len(output.Results) {
		return &exitError{code: 2}
	}
	if output.Failed > 0 {
		return &exitError{code: 1}
	}
	return nil
}

func appendFileContent(prompt string, opts *options) (string, error) {
	if len(opts.files) == 0 {
		return prompt, nil
	}

	maxSize, err := files.ParseSize(opts.maxFileSize)
	if err != nil {
		return "", fmt.Errorf("invalid --max-file-size: %w", err)
	}

	fileContent, err := files.LoadContent(files.Request{
		Files:       opts.files,
		MaxFileSize: maxSize,
		MaxTokens:   opts.maxTokens,
	})
	if err != nil {
		return "", fmt.Errorf("load files: %w", err)
	}

	if fileContent != "" {
		return prompt + "\n\n[FILES]\n" + fileContent, nil
	}
	return prompt, nil
}

func readPrompt(stdin io.Reader, promptFile string) (string, error) {
	if promptFile != "" {
		return readPromptFromFile(promptFile)
	}
	return readPromptFromReader(stdin)
}

func readPromptFromFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrEmptyPrompt
	}
	return string(data), nil
}

func readPromptFromReader(r io.Reader) (string, error) {
	if f, ok := r.(*os.File); ok {
		stat, _ := f.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			return "", fmt.Errorf("no input: use --prompt-file or pipe data to stdin")
		}
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	if len(data) == 0 {
		return "", ErrEmptyPrompt
	}
	return string(data), nil
}

func runInit(cmd *cobra.Command, opts *options) error {
	stderr := cmd.ErrOrStderr()

	useTemplate, _ := cmd.Flags().GetBool("template")

	output, _ := cmd.Flags().GetString("output")
	path := output
	if path == "" {
		path = config.DefaultConfigPath()
	}

	if aborted := confirmOverwrite(cmd, path); aborted {
		return nil
	}

	if useTemplate {
		return runInitTemplate(stderr, path)
	}

	return runInitInteractive(cmd, opts, stderr, path)
}

func confirmOverwrite(cmd *cobra.Command, path string) (aborted bool) {
	if _, err := os.Stat(path); err != nil {
		return false
	}

	stderr := cmd.ErrOrStderr()
	_, _ = fmt.Fprintf(stderr, "File already exists: %s\nOverwrite? [y/N]: ", path)

	reader := bufio.NewReader(cmd.InOrStdin())
	answer, _ := reader.ReadString('\n')
	answer = strings.TrimSpace(strings.ToLower(answer))

	if answer != "y" && answer != "yes" {
		_, _ = fmt.Fprintln(stderr, "Aborted")
		return true
	}
	return false
}

func runInitTemplate(stderr io.Writer, path string) error {
	if err := config.GenerateExample(path); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return errInitConfig
	}
	_, _ = fmt.Fprintf(stderr, "Configuration file created: %s\n", path)
	return nil
}

func runInitInteractive(cmd *cobra.Command, opts *options, stderr io.Writer, path string) error {
	token := getAPIToken(opts.token)
	if token == "" {
		_, _ = fmt.Fprintln(stderr, "Error: API token required: use --token or set OPENROUTER_API_KEY")
		return errInitConfig
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	var verboseW io.Writer
	if opts.verbose {
		verboseW = stderr
	}

	// load existing config for pre-selection (best-effort, ignore errors)
	var existingModels []config.Model
	var preSelected []string
	if existing, err := config.Load(path); err == nil {
		existingModels = existing.Models
		preSelected = extractPreSelected(existing.Models)
	}

	selected, err := modelsel.Run(ctx, token, &modelsel.Options{
		Verbose:     opts.verbose,
		VerboseW:    verboseW,
		PreSelected: preSelected,
	})
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return errInitConfig
	}
	if selected == nil {
		_, _ = fmt.Fprintln(stderr, "Cancelled")
		return nil
	}

	all := mergeDisabledModels(existingModels, selected)
	content := config.GenerateFromModels(all)

	if err := config.WriteConfig(path, content); err != nil {
		_, _ = fmt.Fprintf(stderr, "Error: %v\n", err)
		return errInitConfig
	}

	_, _ = fmt.Fprintf(stderr, "Configuration file created: %s\n", path)
	return nil
}

func extractPreSelected(models []config.Model) []string {
	var ids []string
	for i := range models {
		if models[i].Enabled {
			ids = append(ids, models[i].Model)
		}
	}
	return ids
}

func mergeDisabledModels(existing []config.Model, selected []config.SelectedModel) []config.SelectedModel {
	selectedSet := make(map[string]struct{}, len(selected))
	for _, m := range selected {
		selectedSet[m.ID] = struct{}{}
	}

	result := append([]config.SelectedModel(nil), selected...)
	for i := range existing {
		if _, ok := selectedSet[existing[i].Model]; !ok {
			result = append(result, config.SelectedModel{
				ID:      existing[i].Model,
				Name:    existing[i].Name,
				Enabled: false,
			})
		}
	}
	return result
}
