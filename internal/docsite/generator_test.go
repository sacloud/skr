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

package docsite

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/text"
)

func TestDiscoverCommandPaths(t *testing.T) {
	help := `Usage: skr <command> [flags]

Commands:
  config current
    Print the current profile name.

  iaas-api switch find [flags]
    Find switches.

  eventbus-api trigger create --request=STRING
    Create a trigger.

Run "skr <command> --help" for more information on a command.
`
	got, err := discoverCommandPaths(help)
	if err != nil {
		t.Fatalf("discoverCommandPaths() error = %v", err)
	}
	want := []string{
		"config",
		"config current",
		"iaas-api",
		"iaas-api switch",
		"iaas-api switch find",
		"eventbus-api",
		"eventbus-api trigger",
		"eventbus-api trigger create",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("discoverCommandPaths() = %q, want %q", got, want)
	}
}

func TestDiscoverCommandPathsWithoutCommands(t *testing.T) {
	if _, err := discoverCommandPaths("Usage: skr [flags]\n"); err == nil {
		t.Fatal("discoverCommandPaths() error = nil, want error")
	}
}

func TestManualWebPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "README.md", want: "index.html"},
		{input: "tutorials/README.md", want: "tutorials/index.html"},
		{input: "tutorials/simplemq-api.md", want: "tutorials/simplemq-api.html"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := manualWebPath(tt.input); got != tt.want {
				t.Errorf("manualWebPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRenderMarkdownRewritesLocalMarkdownLinks(t *testing.T) {
	source := []byte("[home](README.md) [tutorial](tutorials/switch.md#step-1) [external](https://example.com/docs.md)")
	got, err := renderMarkdown(source)
	if err != nil {
		t.Fatalf("renderMarkdown() error = %v", err)
	}
	for _, want := range []string{
		`href="index.html"`,
		`href="tutorials/switch.html#step-1"`,
		`href="https://example.com/docs.md"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("renderMarkdown() = %q, want it to contain %q", got, want)
		}
	}
}

func TestRewriteMarkdownLinksRejectsInvalidURL(t *testing.T) {
	document := goldmark.DefaultParser().Parse(text.NewReader([]byte("[link](%zz.md)")))
	if err := rewriteMarkdownLinks(document); err == nil {
		t.Fatal("rewriteMarkdownLinks() error = nil, want error")
	}
}

func TestRelativeURL(t *testing.T) {
	tests := []struct {
		from string
		to   string
		want string
	}{
		{from: "index.html", to: "commands/index.html", want: "commands/index.html"},
		{from: "tutorials/iaas/switch.html", to: "index.html", want: "../../index.html"},
		{from: "commands/index.html", to: "commands/index.html", want: "index.html"},
	}
	for _, tt := range tests {
		t.Run(tt.from+"-"+tt.to, func(t *testing.T) {
			if got := relativeURL(tt.from, tt.to); got != tt.want {
				t.Errorf("relativeURL(%q, %q) = %q, want %q", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
