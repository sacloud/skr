package helper

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

type Result interface {
	Description() string
	Render() string
}

type commandResult struct {
	description string
	args        []string
	stdout      string
	stderr      string
	duration    time.Duration
}

func (c *commandResult) Description() string { return c.description }
func (c *commandResult) Render() string {
	var buf bytes.Buffer
	buf.WriteString("**command**:\n```\n")
	buf.WriteString(strings.Join(append([]string{"skr"}, c.args...), " "))
	buf.WriteString("\n```\n\n")
	buf.WriteString("**duration**:\n")
	buf.WriteString(fmt.Sprintf("%v\n\n", c.duration))
	buf.WriteString("**stdout**:\n```\n")
	buf.WriteString(c.stdout)
	buf.WriteString("\n```\n\n")
	buf.WriteString("**stderr**:\n```\n")
	buf.WriteString(c.stderr)
	buf.WriteString("\n```\n\n")
	return buf.String()
}

type requestResult struct {
	name string
	data string
}

func (r *requestResult) Description() string { return fmt.Sprintf("Create request file: %s", r.name) }
func (r *requestResult) Render() string {
	var buf bytes.Buffer
	buf.WriteString("**request JSON**:\n```json\n")
	buf.WriteString(r.data)
	buf.WriteString("\n```\n\n")
	return buf.String()
}
