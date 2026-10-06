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

	"github.com/sacloud/skr/internal/docsite"
)

func main() {
	cliPath := flag.String("cli", "", "path to the skr executable")
	manualPath := flag.String("manual", "docs/manual", "path to the user manual")
	outputPath := flag.String("out", "_site", "directory for generated HTML")
	flag.Parse()

	if *cliPath == "" {
		fmt.Fprintln(os.Stderr, "-cli is required")
		os.Exit(2)
	}
	if err := docsite.Generate(*cliPath, *manualPath, *outputPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
