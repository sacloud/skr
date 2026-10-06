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

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sacloud/skr/internal/apigen"
)

func main() {
	configPath := flag.String("config", "", "path to the IaaS API generator config")
	outputPath := flag.String("out", "", "path to the generated Go file")
	flag.Parse()

	if *configPath == "" || *outputPath == "" {
		fmt.Fprintln(os.Stderr, "both -config and -out are required")
		os.Exit(2)
	}
	if err := generate(*configPath, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(configPath, outputPath string) error {
	data, err := os.ReadFile(configPath) // #nosec G304 -- The config path is explicitly provided by the local CLI user.
	if err != nil {
		return fmt.Errorf("read config %q: %w", configPath, err)
	}
	config, err := apigen.DecodeConfig(data)
	if err != nil {
		return err
	}
	source, err := apigen.Generate(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, source, 0o644); err != nil { // #nosec G306 G703 -- The local user selects the output path; generated Go source is non-sensitive and must remain readable.
		return fmt.Errorf("write generated Go file %q: %w", outputPath, err)
	}
	return nil
}
