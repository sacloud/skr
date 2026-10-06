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
	"bytes"
	"context"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type commandHelp struct {
	Path string
	ID   string
	Help string
}

type commandGroup struct {
	Name  string
	Items []commandHelp
}

type pageData struct {
	Title        string
	HomeURL      string
	CommandsURL  string
	ManualMarkup template.HTML
}

type commandPageData struct {
	Groups []commandGroup
}

func Generate(cliPath, manualPath, outputPath string) error {
	rootHelp, err := runHelp(cliPath, nil)
	if err != nil {
		return err
	}
	commandPaths, err := discoverCommandPaths(rootHelp)
	if err != nil {
		return err
	}

	commands := []commandHelp{{Path: "skr", ID: "skr", Help: rootHelp}}
	for _, commandPath := range commandPaths {
		help, err := runHelp(cliPath, strings.Fields(commandPath))
		if err != nil {
			return err
		}
		commands = append(commands, commandHelp{
			Path: commandPath,
			ID:   strings.ReplaceAll(commandPath, " ", "-"),
			Help: help,
		})
	}

	if err := renderManual(manualPath, outputPath); err != nil {
		return err
	}
	if err := renderCommandPage(commands, outputPath); err != nil {
		return err
	}
	return nil
}

func runHelp(cliPath string, commandPath []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	args := append(append([]string(nil), commandPath...), "--help")
	output, err := exec.CommandContext(ctx, cliPath, args...).CombinedOutput() //nolint:gosec // cliPath is the explicitly selected skr executable; arguments are fixed help flags and no shell is used.
	if err != nil {
		return "", fmt.Errorf("run skr %s --help: %w\n%s", strings.Join(commandPath, " "), err, output)
	}
	return string(output), nil
}

func discoverCommandPaths(rootHelp string) ([]string, error) {
	var paths []string
	seen := make(map[string]bool)
	inCommands := false
	for _, line := range strings.Split(rootHelp, "\n") {
		if strings.TrimSpace(line) == "Commands:" {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		if strings.HasPrefix(line, "Run ") {
			break
		}
		indent := len(line) - len(strings.TrimLeft(line, " "))
		if indent != 2 {
			continue
		}

		var command []string
		for _, part := range strings.Fields(strings.TrimSpace(line)) {
			if strings.HasPrefix(part, "-") || strings.HasPrefix(part, "<") || strings.HasPrefix(part, "[") {
				break
			}
			command = append(command, part)
		}
		for i := 1; i <= len(command); i++ {
			commandPath := strings.Join(command[:i], " ")
			if !seen[commandPath] {
				seen[commandPath] = true
				paths = append(paths, commandPath)
			}
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("no commands found in skr root help")
	}
	return paths, nil
}

func renderManual(manualPath, outputPath string) error {
	return filepath.WalkDir(manualPath, func(sourcePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			return nil
		}

		source, err := os.ReadFile(sourcePath) //nolint:gosec // WalkDir yields Markdown paths confined to the configured manual tree.
		if err != nil {
			return fmt.Errorf("read manual %s: %w", sourcePath, err)
		}
		relative, err := filepath.Rel(manualPath, sourcePath)
		if err != nil {
			return fmt.Errorf("get relative path for manual %s: %w", sourcePath, err)
		}
		webPath := manualWebPath(filepath.ToSlash(relative))
		rendered, err := renderMarkdown(source)
		if err != nil {
			return fmt.Errorf("render manual %s: %w", sourcePath, err)
		}
		title := strings.TrimSuffix(filepath.Base(sourcePath), filepath.Ext(sourcePath))
		if strings.EqualFold(title, "README") {
			title = "skr 利用者向けドキュメント"
		}
		if err := writePage(outputPath, webPath, title, template.HTML(rendered)); err != nil { //nolint:gosec // Goldmark's default renderer suppresses raw HTML and dangerous links.
			return fmt.Errorf("write manual page %s: %w", webPath, err)
		}
		return nil
	})
}

func manualWebPath(relative string) string {
	if strings.EqualFold(path.Base(relative), "README.md") {
		return path.Join(path.Dir(relative), "index.html")
	}
	return strings.TrimSuffix(relative, path.Ext(relative)) + ".html"
}

func renderMarkdown(source []byte) (string, error) {
	markdown := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	document := markdown.Parser().Parse(text.NewReader(source))
	if err := rewriteMarkdownLinks(document); err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := markdown.Renderer().Render(&output, source, document); err != nil {
		return "", err
	}
	return output.String(), nil
}

func rewriteMarkdownLinks(document ast.Node) error {
	return ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		link, ok := node.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}
		destination, err := url.Parse(string(link.Destination))
		if err != nil {
			return ast.WalkStop, fmt.Errorf("parse link destination %q: %w", link.Destination, err)
		}
		if destination.IsAbs() || destination.Host != "" || !strings.EqualFold(path.Ext(destination.Path), ".md") {
			return ast.WalkContinue, nil
		}
		if strings.EqualFold(path.Base(destination.Path), "README.md") {
			destination.Path = path.Join(path.Dir(destination.Path), "index.html")
		} else {
			destination.Path = strings.TrimSuffix(destination.Path, path.Ext(destination.Path)) + ".html"
		}
		link.Destination = []byte(destination.String())
		return ast.WalkContinue, nil
	})
}

func renderCommandPage(commands []commandHelp, outputPath string) error {
	groupMap := make(map[string]int)
	var groups []commandGroup
	for _, command := range commands {
		groupName := "基本"
		if command.Path != "skr" {
			groupName = strings.Fields(command.Path)[0]
		}
		index, ok := groupMap[groupName]
		if !ok {
			index = len(groups)
			groupMap[groupName] = index
			groups = append(groups, commandGroup{Name: groupName})
		}
		groups[index].Items = append(groups[index].Items, command)
	}

	body, err := renderCommandBody(commandPageData{Groups: groups})
	if err != nil {
		return err
	}
	return writePage(outputPath, "commands/index.html", "CLI コマンドヘルプ", template.HTML(body)) //nolint:gosec // The body is rendered by html/template with all help values escaped.
}

func renderCommandBody(data commandPageData) (string, error) {
	const commandTemplate = `{{define "commands"}}<h1>CLI コマンドヘルプ</h1>
<p>実際の <code>skr --help</code> の出力を掲載しています。コマンド名を選ぶと、対応する <code>--help</code> の内容を確認できます。</p>
<label class="search-label" for="command-filter">コマンドを検索</label>
<input id="command-filter" class="search-input" type="search" placeholder="例: simplemq queue create" autocomplete="off">
<p id="command-count" class="muted" aria-live="polite"></p>
<div id="command-groups">
{{range .Groups}}<section class="command-group">
<h2>{{.Name}}</h2>
{{range .Items}}<details class="command-item" id="{{.ID}}">
<summary><code>skr {{.Path}}</code></summary>
<pre><code>{{.Help}}</code></pre>
</details>{{end}}
</section>{{end}}
</div>
<script>
const filter = document.getElementById("command-filter");
const items = Array.from(document.querySelectorAll(".command-item"));
const groups = Array.from(document.querySelectorAll(".command-group"));
const count = document.getElementById("command-count");
function filterCommands() {
  const query = filter.value.trim().toLocaleLowerCase();
  let visible = 0;
  for (const item of items) {
    item.hidden = !item.textContent.toLocaleLowerCase().includes(query);
    if (!item.hidden) visible++;
  }
  for (const group of groups) {
    group.hidden = !group.querySelector(".command-item:not([hidden])");
  }
  count.textContent = visible + " / " + items.length + " コマンド";
}
filter.addEventListener("input", filterCommands);
filterCommands();
</script>{{end}}`
	tmpl, err := template.New("commands").Parse(commandTemplate)
	if err != nil {
		return "", err
	}
	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "commands", data); err != nil {
		return "", err
	}
	return output.String(), nil
}

func writePage(outputPath, webPath, title string, body template.HTML) error {
	page := pageData{
		Title:        title,
		HomeURL:      relativeURL(webPath, "index.html"),
		CommandsURL:  relativeURL(webPath, "commands/index.html"),
		ManualMarkup: body,
	}
	var output bytes.Buffer
	if err := pageTemplate.Execute(&output, page); err != nil {
		return err
	}
	target := filepath.Join(outputPath, filepath.FromSlash(webPath))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // Published static pages need traversable directories in the Pages artifact.
		return err
	}
	if err := os.WriteFile(target, output.Bytes(), 0o644); err != nil { //nolint:gosec // Generated pages are public documentation and must be readable in the artifact.
		return err
	}
	return nil
}

func relativeURL(fromPage, target string) string {
	relative, err := filepath.Rel(
		filepath.Dir(filepath.FromSlash(fromPage)),
		filepath.FromSlash(target),
	)
	if err != nil {
		return target
	}
	return filepath.ToSlash(relative)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="ja">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} | skr</title>
<style>
:root{color-scheme:light dark;font:16px/1.65 system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;--bg:#fff;--fg:#1f2328;--muted:#59636e;--border:#d1d9e0;--surface:#f6f8fa;--link:#0969da}
@media(prefers-color-scheme:dark){:root{--bg:#0d1117;--fg:#e6edf3;--muted:#9198a1;--border:#30363d;--surface:#161b22;--link:#4493f8}}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--fg)}a{color:var(--link)}.site-header{display:flex;gap:2rem;align-items:center;padding:.85rem max(1rem,calc((100% - 1100px)/2));border-bottom:1px solid var(--border);background:var(--surface)}.brand{font-weight:700;text-decoration:none;font-size:1.15rem}.site-header nav{display:flex;gap:1.25rem;flex-wrap:wrap}.site-header nav a{text-decoration:none}.content{max-width:1100px;margin:2.5rem auto;padding:0 1.25rem 3rem}h1{line-height:1.25;border-bottom:1px solid var(--border);padding-bottom:.5rem}h2{margin-top:2rem}pre{overflow:auto;padding:1rem;border:1px solid var(--border);border-radius:6px;background:var(--surface);line-height:1.5}code{font-family:ui-monospace,SFMono-Regular,Consolas,monospace}blockquote{margin-left:0;padding:.25rem 1rem;border-left:4px solid var(--border);color:var(--muted)}table{border-collapse:collapse;display:block;max-width:100%;overflow:auto}th,td{border:1px solid var(--border);padding:.4rem .65rem}th{background:var(--surface)}img{max-width:100%}.search-label{display:block;font-weight:600;margin:1rem 0 .35rem}.search-input{width:min(100%,40rem);padding:.7rem .8rem;border:1px solid var(--border);border-radius:6px;background:var(--bg);color:var(--fg);font:inherit}.muted{color:var(--muted)}.command-group{margin:1.5rem 0}.command-group h2{font-size:1.25rem}.command-item{margin:.5rem 0;border:1px solid var(--border);border-radius:6px}.command-item summary{cursor:pointer;padding:.6rem .8rem;font-weight:600}.command-item pre{margin:0;border:0;border-top:1px solid var(--border);border-radius:0 0 6px 6px}.command-item[hidden],.command-group[hidden]{display:none}
.development-banner{display:flex;align-items:center;gap:.75rem;padding:.75rem max(1rem,calc((100% - 1100px)/2));background:#fff4e5;color:#7d3600;border-bottom:2px solid #d98b00}.development-label{flex:none;font-weight:700}@media(prefers-color-scheme:dark){.development-banner{background:#2b2113;color:#f0c36d;border-color:#9e6a03}}@media(max-width:600px){.development-banner{align-items:flex-start;flex-direction:column;gap:.25rem}}
</style>
</head>
<body>
<header class="site-header">
<a class="brand" href="{{.HomeURL}}">skr</a>
<nav aria-label="メインナビゲーション">
<a href="{{.HomeURL}}">利用者向けドキュメント</a>
<a href="{{.CommandsURL}}">CLI コマンドヘルプ</a>
</nav>
</header>
<aside class="development-banner" aria-label="開発中バージョン">
<span class="development-label">🚧 開発中バージョン</span>
<span>コマンドや設定、入出力は変更されることがあります。既存の usacloud との互換性は保証しません。</span>
</aside>
<main class="content">{{.ManualMarkup}}</main>
</body>
</html>`))
