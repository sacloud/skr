// Copyright 2022-2026 The sacloud/skr Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package evidence

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Recorder struct {
	dir      string
	sequence int
}

func New(service string) (*Recorder, error) {
	if filepath.Base(service) != service || service == "" {
		return nil, fmt.Errorf("invalid E2E evidence service %q", service)
	}
	command := exec.Command("git", "rev-parse", "--show-toplevel")
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("find repository root for E2E evidence: %w", err)
	}
	root := strings.TrimSpace(string(output))
	if root == "" {
		return nil, errors.New("find repository root for E2E evidence: empty path")
	}
	return CreateAt(filepath.Join(root, "tmp", service), time.Now())
}

func CreateAt(baseDir string, now time.Time) (*Recorder, error) {
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		return nil, fmt.Errorf("create E2E evidence root: %w", err)
	}
	prefix := now.Local().Format("200601021504")
	for suffix := 1; ; suffix++ {
		name := prefix
		if suffix > 1 {
			name = fmt.Sprintf("%s-%02d", prefix, suffix)
		}
		dir := filepath.Join(baseDir, name)
		if err := os.Mkdir(dir, 0o700); err != nil {
			if errors.Is(err, os.ErrExist) {
				continue
			}
			return nil, fmt.Errorf("create E2E evidence directory %s: %w", dir, err)
		}
		order := "Sequence\tStep\tResult\tFile\n"
		if err := os.WriteFile(filepath.Join(dir, "ORDER.txt"), []byte(order), 0o600); err != nil {
			return nil, fmt.Errorf("initialize E2E evidence order in %s: %w", dir, err)
		}
		return &Recorder{dir: dir}, nil
	}
}

func (r *Recorder) Dir() string {
	return r.dir
}

func ValidateStep(step string) error {
	if filepath.Base(step) != step || step == "" {
		return fmt.Errorf("invalid evidence step %q", step)
	}
	return nil
}

func (r *Recorder) Record(step string, args []string, request any, stdout, stderr string, runErr error, redactStdout bool) error {
	if err := ValidateStep(step); err != nil {
		return err
	}
	r.sequence++
	sequence := r.sequence
	filename := fmt.Sprintf("%03d-%s.json", sequence, step)
	record := struct {
		Sequence int      `json:"sequence"`
		Step     string   `json:"step"`
		Args     []string `json:"args,omitempty"`
		Request  any      `json:"request,omitempty"`
		Stdout   string   `json:"stdout"`
		Stderr   string   `json:"stderr"`
		Error    string   `json:"error,omitempty"`
	}{
		Sequence: sequence,
		Step:     step,
		Args:     args,
		Request:  request,
		Stdout:   stdout,
		Stderr:   stderr,
	}
	if redactStdout {
		record.Stdout = "[REDACTED: APIKey]"
	}
	if runErr != nil {
		record.Error = runErr.Error()
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("%s: encode evidence: %w", step, err)
	}
	if err := os.WriteFile(filepath.Join(r.dir, filename), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("%s: write evidence: %w", step, err)
	}
	result := "ok"
	if runErr != nil {
		result = "failed"
	}
	return r.appendOrder(sequence, step, result, filename)
}

func (r *Recorder) SetResult(result string, completed time.Time) error {
	if result != "passed" && result != "failed" {
		return fmt.Errorf("invalid E2E result %q", result)
	}
	content := fmt.Sprintf("Result: %s\nCompleted: %s\n", result, completed.Format(time.RFC3339))
	if err := os.WriteFile(filepath.Join(r.dir, "RESULT.txt"), []byte(content), 0o600); err != nil {
		return fmt.Errorf("write E2E result in %s: %w", r.dir, err)
	}
	return nil
}

func (r *Recorder) appendOrder(sequence int, step, result, filename string) error {
	path := filepath.Join(r.dir, "ORDER.txt")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600) //nolint:gosec // The filename is fixed inside the private evidence directory.
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(file, "%03d\t%s\t%s\t%s\n", sequence, step, result, filename); err != nil {
		return errors.Join(fmt.Errorf("append %s: %w", path, err), file.Close())
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}
