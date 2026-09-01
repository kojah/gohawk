package main

import (
	"fmt"
	"html"
	"strings"
)

// groupIntros carries each group's one-sentence introduction, shown under its
// heading on the catalog page. Keyed by group name.
var groupIntros = map[string]string{
	"contracts":   "These analyzers make contracts visible in Go types and APIs, where callers and tools can rely on them.",
	"ownership":   "These analyzers look for work or resources whose owner cannot be identified on every relevant path.",
	"reliability": "These analyzers cover failure modes that often survive ordinary type checking and code review.",
	"testing":     "These analyzers keep test failures bounded and make helper behavior visible at the call site.",
}

func analyzerIndex(data manifest) string {
	var output strings.Builder
	output.WriteString("---\ntitle: All analyzers\ndescription: The gohawk analyzer catalog, generated from the registered Go analyzers.\n---\n\n")
	output.WriteString("<!-- Run go generate ./... to update this page; do not edit it by hand. -->\n\n")
	output.WriteString("gohawk ships a focused set of analyzers rather than a general-purpose lint\n")
	output.WriteString("catalog. Every check identifies the kind of claim it makes:\n\n")
	output.WriteString("- **Defect** means the available evidence establishes broken or ineffective behavior.\n")
	output.WriteString("- **Hazard** means the behavior is risky, but harm depends on a wider runtime contract.\n")
	output.WriteString("- **Policy** means valid Go violates an intentionally selected engineering convention.\n\n")
	output.WriteString("Kind is descriptive metadata and does not change whether a check is enabled by default.\n")
	for _, group := range data.Groups {
		fmt.Fprintf(&output, "\n## %s\n\n", group.Title)
		if intro := groupIntros[group.Name]; intro != "" {
			output.WriteString(intro + "\n\n")
		}
		output.WriteString(groupCards(group))
		output.WriteByte('\n')
	}
	return output.String()
}

// groupCards renders a group's analyzers as a grid of linked cards. The output
// is raw HTML because Markdown tables cannot carry the card layout; anything
// inside it is therefore escaped here rather than by the Markdown renderer.
func groupCards(group group) string {
	var output strings.Builder
	output.WriteString(`<div class="analyzer-grid">` + "\n")
	for _, analyzer := range group.Analyzers {
		link := group.Slug + "/" + analyzer.Name + "/"
		fmt.Fprintf(&output, "  "+`<a class="analyzer-card" href="%s">`+"\n", html.EscapeString(link))
		fmt.Fprintf(&output, "    "+`<span class="analyzer-name">%s</span>`+"\n", html.EscapeString(analyzer.Name))
		fmt.Fprintf(&output, "    "+`<span class="analyzer-detects">%s</span>`+"\n", inlineCode(analyzer.Summary))
		output.WriteString("  </a>\n")
	}
	output.WriteString("</div>")
	return output.String()
}

// inlineCode escapes text for HTML and renders `backtick` spans as code, which
// the Markdown renderer would otherwise leave alone inside a raw HTML block.
func inlineCode(text string) string {
	var output strings.Builder
	for index, segment := range strings.Split(text, "`") {
		if index%2 == 1 {
			output.WriteString("<code>" + html.EscapeString(segment) + "</code>")
			continue
		}
		output.WriteString(html.EscapeString(segment))
	}
	return output.String()
}

func optionsTable(options []optionFlag) string {
	var output strings.Builder
	output.WriteString("| Knob | Default | Effect |\n| --- | --- | --- |\n")
	for _, option := range options {
		defaultValue := option.Default
		if defaultValue == "" {
			defaultValue = "empty"
		} else {
			defaultValue = "`" + strings.ReplaceAll(defaultValue, "`", "\\`") + "`"
		}
		fmt.Fprintf(&output, "| `%s` | %s | %s |\n", option.Name, defaultValue, option.Usage)
	}
	return strings.TrimSuffix(output.String(), "\n")
}
