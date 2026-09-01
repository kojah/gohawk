package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	gohawk "github.com/kojah/gohawk/analyzers"
	"github.com/kojah/gohawk/internal/docexamples"
)

func collectManifest(root string) (manifest, error) {
	metadata := gohawk.AnalyzerMetadata()
	seen := make(map[string]bool)
	analyzerGroups := gohawk.AnalyzerGroups()
	targets := make([]docexamples.Target, 0, len(metadata))
	for _, analyzerGroup := range analyzerGroups {
		for _, registered := range analyzerGroup.Analyzers {
			if seen[registered.Name] {
				return manifest{}, fmt.Errorf("analyzer %q is registered more than once", registered.Name)
			}
			seen[registered.Name] = true
			if _, ok := metadata[registered.Name]; !ok {
				return manifest{}, fmt.Errorf("analyzer %q has no metadata", registered.Name)
			}
			targets = append(targets, docexamples.Target{
				TestRoot: filepath.Join(root, "internal", "analyzers", analyzerGroup.Name, registered.Name, "testdata"),
				Analyzer: registered,
			})
		}
	}
	for name := range metadata {
		if !seen[name] {
			return manifest{}, fmt.Errorf("metadata exists for unknown analyzer %q", name)
		}
	}
	examples, err := docexamples.CollectAll(targets)
	if err != nil {
		return manifest{}, err
	}

	result := manifest{}
	for _, analyzerGroup := range analyzerGroups {
		group := group{
			Name:  analyzerGroup.Name,
			Title: heading(analyzerGroup.Doc),
			Slug:  analyzerGroup.DocPath,
		}
		for _, registered := range analyzerGroup.Analyzers {
			info := metadata[registered.Name]
			item := analyzer{
				Name:         registered.Name,
				Summary:      sentence(registered.Doc),
				Path:         "analyzers/" + group.Slug + "/" + registered.Name,
				OptIn:        info.OptIn,
				Checks:       checkManifest(info.OptIn, info.Checks),
				SuggestedFix: info.SuggestedFix,
				Options:      []optionFlag{},
				Examples:     examples[registered.Name],
			}
			registered.Flags.VisitAll(func(value *flag.Flag) {
				item.Options = append(item.Options, optionFlag{
					Name:    value.Name,
					Default: value.DefValue,
					Usage:   sentence(value.Usage),
				})
			})
			group.Analyzers = append(group.Analyzers, item)
		}
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

func checkManifest(analyzerOptIn bool, checks []gohawk.AnalyzerCheckInfo) []check {
	result := make([]check, len(checks))
	for index, item := range checks {
		result[index] = check{ID: string(item.ID), Summary: item.Doc, Kind: item.Kind, OptIn: analyzerOptIn || item.OptIn}
	}
	return result
}

func sentence(value string) string {
	value = heading(value)
	if value == "" {
		return value
	}
	if !strings.HasSuffix(value, ".") {
		value += "."
	}
	return value
}

func heading(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
