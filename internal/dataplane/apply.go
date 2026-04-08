/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dataplane

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/voronkov44/k8s-lb-controller/internal/provider"
)

const configPlaceholder = "{{config}}"

// ApplyConfig contains runtime settings for HAProxy config materialization.
type ApplyConfig struct {
	ConfigPath      string
	ValidateCommand string
	ReloadCommand   string
}

// Applier renders and atomically applies one aggregate HAProxy config file.
type Applier struct {
	config ApplyConfig
}

// NewApplier creates an Applier with validated runtime settings.
func NewApplier(cfg ApplyConfig) (*Applier, error) {
	configPath := strings.TrimSpace(cfg.ConfigPath)
	if configPath == "" {
		return nil, fmt.Errorf("config path must not be empty")
	}

	return &Applier{
		config: ApplyConfig{
			ConfigPath:      filepath.Clean(configPath),
			ValidateCommand: strings.TrimSpace(cfg.ValidateCommand),
			ReloadCommand:   strings.TrimSpace(cfg.ReloadCommand),
		},
	}, nil
}

// Apply renders the full config for the provided Services and writes it atomically when it changed.
func (a *Applier) Apply(ctx context.Context, services []provider.Service) (bool, error) {
	renderedConfig, err := Render(services)
	if err != nil {
		return false, fmt.Errorf("render HAProxy config: %w", err)
	}

	return a.applyRenderedConfig(ctx, renderedConfig)
}

func (a *Applier) applyRenderedConfig(ctx context.Context, renderedConfig string) (bool, error) {
	upToDate, err := renderedConfigMatchesCurrentFile(a.config.ConfigPath, renderedConfig)
	if err != nil {
		return false, err
	}
	if upToDate {
		return false, nil
	}

	candidatePath, cleanupCandidate, err := writeCandidateFile(a.config.ConfigPath, []byte(renderedConfig))
	if err != nil {
		return false, fmt.Errorf("write candidate config: %w", err)
	}
	applied := false
	defer func() {
		if !applied {
			cleanupCandidate()
		}
	}()

	if err := runCommand(ctx, a.config.ValidateCommand, candidatePath); err != nil {
		return false, fmt.Errorf("validate config: %w", err)
	}

	if err := os.Rename(candidatePath, a.config.ConfigPath); err != nil {
		return false, fmt.Errorf("replace active config %q: %w", a.config.ConfigPath, err)
	}
	applied = true

	if err := syncPath(filepath.Dir(a.config.ConfigPath)); err != nil {
		return false, fmt.Errorf("sync config directory: %w", err)
	}

	if err := runCommand(ctx, a.config.ReloadCommand, a.config.ConfigPath); err != nil {
		return false, fmt.Errorf("reload HAProxy: %w", err)
	}

	return true, nil
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

func renderedConfigMatchesCurrentFile(configPath, renderedConfig string) (bool, error) {
	currentConfig, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, fmt.Errorf("read active config %q: %w", configPath, err)
	}

	return string(currentConfig) == renderedConfig, nil
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
