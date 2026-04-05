package haproxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"

	"github.com/f1lzz/k8s-lb-controller/internal/provider"
)

// Provider stores desired Service state in memory and materializes it as an HAProxy config file.
type Provider struct {
	mu       sync.Mutex
	config   Config
	services map[provider.ServiceRef]provider.Service
}

// NewProvider creates a file-based HAProxy provider.
func NewProvider(cfg Config) (*Provider, error) {
	configPath := strings.TrimSpace(cfg.ConfigPath)
	if configPath == "" {
		return nil, fmt.Errorf("config path must not be empty")
	}

	return &Provider{
		config: Config{
			ConfigPath:      filepath.Clean(configPath),
			ValidateCommand: strings.TrimSpace(cfg.ValidateCommand),
			ReloadCommand:   strings.TrimSpace(cfg.ReloadCommand),
		},
		services: make(map[provider.ServiceRef]provider.Service),
	}, nil
}

// Ensure upserts a Service entry, rewrites the aggregate config, and optionally validates/reloads it.
func (p *Provider) Ensure(ctx context.Context, service provider.Service) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextState := cloneServices(p.services)
	nextState[service.Ref()] = service.DeepCopy()

	if err := p.apply(ctx, nextState); err != nil {
		return err
	}

	p.services = nextState
	return nil
}

// Delete removes a Service entry, rewrites the aggregate config, and succeeds even when the entry is absent.
func (p *Provider) Delete(ctx context.Context, ref provider.ServiceRef) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	nextState := cloneServices(p.services)
	delete(nextState, ref)

	if err := p.apply(ctx, nextState); err != nil {
		return err
	}

	p.services = nextState
	return nil
}

func (p *Provider) apply(ctx context.Context, services map[provider.ServiceRef]provider.Service) error {
	renderedConfig, err := Render(servicesToList(services))
	if err != nil {
		return err
	}

	candidatePath, cleanupCandidate, err := writeCandidateFile(p.config.ConfigPath, []byte(renderedConfig))
	if err != nil {
		return fmt.Errorf("write candidate config: %w", err)
	}
	applied := false
	defer func() {
		if !applied {
			cleanupCandidate()
		}
	}()

	if err := runCommand(ctx, p.config.ValidateCommand, candidatePath); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	if err := os.Rename(candidatePath, p.config.ConfigPath); err != nil {
		return fmt.Errorf("replace active config %q: %w", p.config.ConfigPath, err)
	}
	applied = true

	if err := syncPath(filepath.Dir(p.config.ConfigPath)); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}

	if err := runCommand(ctx, p.config.ReloadCommand, p.config.ConfigPath); err != nil {
		return fmt.Errorf("reload HAProxy: %w", err)
	}

	return nil
}

func writeCandidateFile(configPath string, data []byte) (string, func(), error) {
	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create config directory %q: %w", configDir, err)
	}

	candidateFile, err := os.CreateTemp(configDir, filepath.Base(configPath)+".tmp-*")
	if err != nil {
		return "", nil, fmt.Errorf("create candidate file: %w", err)
	}

	candidatePath := candidateFile.Name()
	cleanup := func() {
		_ = os.Remove(candidatePath)
	}

	if _, err := candidateFile.Write(data); err != nil {
		_ = candidateFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("write candidate file: %w", err)
	}

	if err := candidateFile.Sync(); err != nil {
		_ = candidateFile.Close()
		cleanup()
		return "", nil, fmt.Errorf("sync candidate file: %w", err)
	}

	if err := candidateFile.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("close candidate file: %w", err)
	}

	return candidatePath, cleanup, nil
}

func runCommand(ctx context.Context, command, configPath string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	replaced := strings.ReplaceAll(command, configPlaceholder, configPath)
	args := strings.Fields(replaced)
	if len(args) == 0 {
		return fmt.Errorf("empty command after parsing %q", command)
	}

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmedOutput := strings.TrimSpace(string(output))
		if trimmedOutput == "" {
			return err
		}

		return fmt.Errorf("%w: %s", err, trimmedOutput)
	}

	return nil
}

func syncPath(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close()
	}()

	if err := directory.Sync(); err != nil && !ignorableSyncError(err) {
		return err
	}

	return nil
}

func ignorableSyncError(err error) bool {
	return errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP)
}

func cloneServices(services map[provider.ServiceRef]provider.Service) map[provider.ServiceRef]provider.Service {
	cloned := make(map[provider.ServiceRef]provider.Service, len(services))
	for ref, service := range services {
		cloned[ref] = service.DeepCopy()
	}

	return cloned
}

func servicesToList(services map[provider.ServiceRef]provider.Service) []provider.Service {
	list := make([]provider.Service, 0, len(services))
	for _, service := range services {
		list = append(list, service.DeepCopy())
	}

	slices.SortFunc(list, func(a, b provider.Service) int {
		if a.Namespace != b.Namespace {
			return strings.Compare(a.Namespace, b.Namespace)
		}

		return strings.Compare(a.Name, b.Name)
	})

	return list
}

var _ provider.Provider = (*Provider)(nil)
