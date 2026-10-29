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

// apigen-wiring は、API コマンド生成設定から internal/cli 配下の配線コードを生成します (ADR 0068)。
// 1 回の実行で複数の設定を処理し、配線コードを 1 ファイルに出力します。
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sacloud/skr/internal/apigen"
)

func main() {
	mode := flag.String("mode", "", "generator mode: iaas or ogen")
	outputPath := flag.String("out", "", "path to the generated Go file")
	flag.Parse()

	if *mode == "" || *outputPath == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: apigen-wiring -mode iaas|ogen -out internal/cli/<domain>_wiring_generated.go CONFIG...")
		os.Exit(2)
	}
	var generatorMode apigen.Mode
	switch *mode {
	case "iaas":
		generatorMode = apigen.ModeIaaS
	case "ogen":
		generatorMode = apigen.ModeOgen
	default:
		fmt.Fprintf(os.Stderr, "unknown mode: %s\n", *mode)
		os.Exit(2)
	}
	if err := apigen.GenerateWiringFile(flag.Args(), *outputPath, generatorMode); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
