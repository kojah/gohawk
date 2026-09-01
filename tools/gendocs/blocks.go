package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/kojah/gohawk/internal/docexamples"
)

func synchronizeChecks(contents []byte, block string) ([]byte, error) {
	if bytes.Contains(contents, []byte(generatedChecksStart)) {
		return replaceGeneratedBlock(contents, generatedChecksStart, generatedChecksEnd, block)
	}
	if bytes.Contains(contents, []byte("\n### Checks\n")) {
		return nil, errors.New("checks subsection exists without generated block markers")
	}
	const heading = "\n## What it detects\n"
	start := bytes.Index(contents, []byte(heading))
	if start < 0 {
		return nil, errors.New("missing What it detects section")
	}
	bodyStart := start + len(heading)
	end := bytes.Index(contents[bodyStart:], []byte("\n## "))
	if end < 0 {
		return nil, errors.New("what it detects must be followed by another section")
	}
	end += bodyStart
	section := []byte("\n### Checks\n\n" + generatedChecksStart + "\n" + block + "\n" + generatedChecksEnd + "\n")
	result := make([]byte, 0, len(contents)+len(section))
	result = append(result, contents[:end]...)
	result = append(result, section...)
	result = append(result, contents[end:]...)
	return result, nil
}

func synchronizeAnalyzerComponents(contents []byte) ([]byte, error) {
	if bytes.Contains(contents, []byte(analyzerComponentImports)) {
		return contents, nil
	}
	if !bytes.HasPrefix(contents, []byte("---\n")) {
		return nil, errors.New("missing frontmatter")
	}
	frontmatterEnd := bytes.Index(contents[len("---\n"):], []byte("\n---\n"))
	if frontmatterEnd < 0 {
		return nil, errors.New("unterminated frontmatter")
	}
	frontmatterEnd += len("---\n") + len("\n---\n")

	result := make([]byte, 0, len(contents)+len(analyzerComponentImports)+2)
	result = append(result, contents[:frontmatterEnd]...)
	result = append(result, '\n')
	result = append(result, analyzerComponentImports...)
	result = append(result, '\n')
	result = append(result, contents[frontmatterEnd:]...)
	return result, nil
}

// checksBlock renders stable check identifiers and summaries as a standard
// Markdown table. The MDX component provides shared opt-in styling while
// preserving Starlight's native table rendering.
func checksBlock(analyzerName string, checks []check) (string, error) {
	var output strings.Builder
	hasOptIn := false
	output.WriteString("| Check | Kind | What it detects |\n")
	output.WriteString("| --- | --- | --- |\n")
	for _, item := range checks {
		localID, ok := strings.CutPrefix(item.ID, analyzerName+"/")
		if !ok || localID == "" {
			return "", fmt.Errorf("check ID %q does not use analyzer prefix %q", item.ID, analyzerName+"/")
		}
		optIn := ""
		if item.OptIn {
			optIn = " optIn"
			hasOptIn = true
		}
		fmt.Fprintf(
			&output,
			"| <CheckIdentity name=\"%s\"%s /> | %s | %s |\n",
			html.EscapeString(localID),
			optIn,
			item.Kind,
			markdownTableCell(item.Summary),
		)
	}
	if hasOptIn {
		output.WriteString("\n\\* Opt-in; requires explicit selection.\n")
	}
	return strings.TrimSuffix(output.String(), "\n"), nil
}

func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\r\n", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.ReplaceAll(value, "|", "\\|")
}

func synchronizeExamples(contents []byte, examples string) ([]byte, error) {
	if bytes.Contains(contents, []byte(generatedExamplesStart)) {
		updated, err := replaceGeneratedBlock(contents, generatedExamplesStart, generatedExamplesEnd, examples)
		if err != nil {
			return nil, err
		}
		doubleBlank := []byte("\n## Examples\n\n\n" + generatedExamplesStart)
		singleBlank := []byte("\n## Examples\n\n" + generatedExamplesStart)
		return bytes.Replace(updated, doubleBlank, singleBlank, 1), nil
	}
	const heading = "\n## Examples\n"
	start := bytes.Index(contents, []byte(heading))
	if start < 0 {
		return nil, errors.New("missing Examples section")
	}
	bodyStart := start + len(heading)
	end := bytes.Index(contents[bodyStart:], []byte("\n## "))
	if end < 0 {
		end = len(contents)
	} else {
		end += bodyStart
	}
	replacement := []byte("\n" + generatedExamplesStart + "\n" + examples + "\n" + generatedExamplesEnd + "\n")
	result := make([]byte, 0, len(contents)-(end-bodyStart)+len(replacement))
	result = append(result, contents[:bodyStart]...)
	result = append(result, replacement...)
	result = append(result, contents[end:]...)
	return result, nil
}

func examplesBlock(examples docexamples.Set) (string, error) {
	var result strings.Builder
	result.WriteString("### Flagged code\n")
	for index, example := range examples.Flagged {
		result.WriteString("\n")
		if len(examples.Flagged) > 1 {
			title := example.Title
			if title == "" {
				title = fmt.Sprintf("Case %d", index+1)
			}
			fmt.Fprintf(&result, "#### %s\n\n", title)
		}
		metadata, err := json.Marshal(example.Diagnostics)
		if err != nil {
			return "", err
		}
		encoded := base64.RawURLEncoding.EncodeToString(metadata)
		fmt.Fprintf(&result, "```go gohawk=\"%s\"\n%s\n```\n", encoded, example.Code)
	}
	fmt.Fprintf(&result, "\n### Accepted code\n\n```go\n%s\n```", examples.OK.Code)
	return result.String(), nil
}

func replaceGeneratedBlock(contents []byte, start, end, body string) ([]byte, error) {
	startIndex := bytes.Index(contents, []byte(start))
	endIndex := bytes.Index(contents, []byte(end))
	if startIndex < 0 || endIndex < 0 || endIndex < startIndex {
		return nil, fmt.Errorf("missing generated block markers %q and %q", start, end)
	}
	endIndex += len(end)
	replacement := []byte(start + "\n" + body + "\n" + end)
	result := make([]byte, 0, len(contents)-endIndex+startIndex+len(replacement))
	result = append(result, contents[:startIndex]...)
	result = append(result, replacement...)
	result = append(result, contents[endIndex:]...)
	return result, nil
}

func synchronizeOptions(contents []byte, table string) ([]byte, error) {
	table = strings.TrimSpace(table)
	if bytes.Contains(contents, []byte(generatedOptionsStart)) {
		return replaceGeneratedBlock(contents, generatedOptionsStart, generatedOptionsEnd, table)
	}
	if bytes.Contains(contents, []byte("\n## Options\n")) {
		return nil, errors.New("options section exists without generated block markers")
	}
	contents = bytes.TrimRight(contents, "\n")
	contents = append(contents, []byte("\n\n## Options\n\n"+generatedOptionsStart+"\n"+table+"\n"+generatedOptionsEnd+"\n")...)
	return contents, nil
}

func hasFrontmatterTitle(contents []byte, title string) bool {
	lines := strings.Split(string(contents), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return false
	}
	for _, line := range lines[1:] {
		if line == "---" {
			break
		}
		if strings.TrimSpace(line) == "title: "+title {
			return true
		}
	}
	return false
}
