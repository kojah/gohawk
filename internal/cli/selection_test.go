package cli

import (
	"slices"
	"strings"
	"testing"

	gohawk "github.com/kojah/gohawk/analyzers"
)

func TestWithAnalyzerSelection(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	selectArguments := func(arguments []string) []string {
		t.Helper()
		selected, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false)
		if err != nil {
			t.Fatal(err)
		}
		return selected
	}

	selected := strings.Join(selectArguments([]string{"gohawk", "-enable=wirepolicy", "./..."}), " ")
	if !strings.Contains(selected, "-wirepolicy=true") || strings.Contains(selected, "-enable=") {
		t.Fatalf("selected arguments = %s", selected)
	}
	help := []string{"gohawk", "help", "wirepolicy"}
	if got := selectArguments(help); !slices.Equal(got, help) {
		t.Fatalf("help arguments = %v, want %v", got, help)
	}
	all := selectArguments([]string{"gohawk", "-enable-all", "./..."})
	for _, analyzer := range analyzers {
		if !slices.Contains(all, "-"+analyzer.Name+"=true") {
			t.Errorf("enable-all arguments do not select %s: %v", analyzer.Name, all)
		}
	}

	got := selectArguments([]string{"gohawk", "-disable=oncepolicy", "./..."})
	joined := strings.Join(got, " ")
	for _, value := range []string{"-contextpolicy=true", "-syncmapatomicity=true"} {
		if !strings.Contains(joined, value) {
			t.Errorf("default arguments do not contain %q: %v", value, got)
		}
	}
	for _, value := range []string{"-oncepolicy=true", "-determinism=true", "-wirepolicy=true", "-globalstate=true"} {
		if strings.Contains(joined, value) {
			t.Errorf("default arguments unexpectedly contain %q: %v", value, got)
		}
	}
}

func TestAnalyzerGroupSelection(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	selectArguments := func(arguments []string) []string {
		t.Helper()
		selected, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false)
		if err != nil {
			t.Fatal(err)
		}
		return selected
	}

	t.Run("groups include opt-in analyzers", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups=testing,contracts", "./..."}), " ")
		for _, value := range []string{
			"-apishape=true", "-contextpolicy=true", "-closedomain=true", "-wirepolicy=true", "-testlifecycle=true", "-testpolicy=true",
		} {
			if !strings.Contains(got, value) {
				t.Errorf("group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-oncepolicy=true", "-channelownership=true", "-enable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("groups combine with individual selection and exclusion", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups", "ownership", "-enable=wirepolicy", "-disable=channelownership", "./..."}), " ")
		for _, value := range []string{"-cancellationownership=true", "-goroutineownership=true", "-wirepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("combined arguments do not contain %q: %s", value, got)
			}
		}
		if strings.Contains(got, "-channelownership=true") {
			t.Errorf("explicit exclusion did not remove channelownership: %s", got)
		}
	})

	t.Run("disabled groups subtract from defaults and allow individual overrides", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-disable-groups=reliability", "-enable=oncepolicy", "./..."}), " ")
		for _, value := range []string{"-contextpolicy=true", "-channelsafety=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("disabled-group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-concurrentcapture=true", "-errorclassification=true", "-disable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("disabled-group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("disabled groups subtract from enable-all", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-all", "-disable-groups=testing", "./..."}), " ")
		for _, value := range []string{"-wirepolicy=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("enable-all exclusion does not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-testpolicy=true", "-disable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("enable-all exclusion unexpectedly contains %q: %s", value, got)
			}
		}
	})
}

func TestInvalidAnalyzerSelection(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()

	t.Run("invalid groups", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-enable-groups=unknown", "./..."},
			{"gohawk", "-enable-groups=testing,testing", "./..."},
			{"gohawk", "-enable-groups=testing,", "./..."},
			{"gohawk", "-enable-groups="},
			{"gohawk", "-enable-groups"},
			{"gohawk", "-disable-groups=reliability,reliability", "./..."},
			{"gohawk", "-disable-groups=unknown", "./..."},
			{"gohawk", "-enable-groups=testing", "-disable-groups=testing", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("arguments %v unexpectedly succeeded", arguments)
			}
		}
	})

	t.Run("invalid analyzer lists", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-enable=unknown", "./..."},
			{"gohawk", "-enable=wirepolicy,wirepolicy", "./..."},
			{"gohawk", "-disable=oncepolicy,oncepolicy", "./..."},
			{"gohawk", "-enable=wirepolicy", "-disable=wirepolicy", "./..."},
			{"gohawk", "-enable="},
			{"gohawk", "-disable"},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("arguments %v unexpectedly succeeded", arguments)
			}
		}
	})

	t.Run("conflicts are reported deterministically", func(t *testing.T) {
		_, err := withAnalyzerSelection(
			[]string{
				"gohawk",
				"-enable=oncepolicy,contextpolicy",
				"-disable=oncepolicy,contextpolicy",
				"./...",
			},
			analyzers,
			groups,
			metadata,
			false,
		)
		if got, want := err.Error(), `analyzer "contextpolicy" cannot be both enabled and disabled`; got != want {
			t.Fatalf("conflict error = %q, want %q", got, want)
		}

		_, err = withAnalyzerSelection(
			[]string{
				"gohawk",
				"-enable-groups=testing,ownership",
				"-disable-groups=testing,ownership",
				"./...",
			},
			analyzers,
			groups,
			metadata,
			false,
		)
		if got, want := err.Error(), `analyzer group "ownership" cannot be both enabled and disabled`; got != want {
			t.Fatalf("group conflict error = %q, want %q", got, want)
		}
	})

	t.Run("legacy analyzer Boolean flags", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-wirepolicy", "./..."},
			{"gohawk", "-oncepolicy=false", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("legacy arguments %v unexpectedly succeeded", arguments)
			}
		}
		if _, err := withAnalyzerSelection([]string{"gohawk", "-wirepolicy=true", "./..."}, analyzers, groups, metadata, true); err != nil {
			t.Fatalf("internal analyzer selection failed: %v", err)
		}
	})
}

func TestRequestedDisabledChecks(t *testing.T) {
	metadata := gohawk.AnalyzerMetadata()

	disabled, remaining, err := requestedDisabledChecks([]string{
		"gohawk",
		"-disable-checks=contextpolicy/context-first,contextpolicy/nil-context",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"contextpolicy/context-first", "contextpolicy/nil-context"} {
		if !disabled[check] {
			t.Errorf("disabled checks do not contain %q: %v", check, disabled)
		}
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-disable-checks=unknown/check", "./..."},
		{"gohawk", "-disable-checks=contextpolicy/context-first,contextpolicy/context-first", "./..."},
		{"gohawk", "-disable-checks=contextpolicy/context-first,", "./..."},
		{"gohawk", "-disable-checks="},
		{"gohawk", "-disable-checks"},
	} {
		if _, _, err := requestedDisabledChecks(arguments, metadata); err == nil {
			t.Errorf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func TestRequestedChecks(t *testing.T) {
	metadata := gohawk.AnalyzerMetadata()
	requested, remaining, err := requestedChecks([]string{
		"gohawk",
		"-enable-checks=testlifecycle/context-root,contextpolicy/nil-context",
		"-disable-checks=contextpolicy/context-first",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"testlifecycle/context-root", "contextpolicy/nil-context"} {
		if !requested.enabled[check] {
			t.Errorf("enabled checks do not contain %q: %v", check, requested.enabled)
		}
	}
	if !requested.disabled["contextpolicy/context-first"] {
		t.Errorf("disabled checks = %v", requested.disabled)
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-enable-checks=unknown/check", "./..."},
		{"gohawk", "-enable-checks=testlifecycle/context-root,testlifecycle/context-root", "./..."},
		{"gohawk", "-enable-checks=testlifecycle/context-root,", "./..."},
		{"gohawk", "-enable-checks="},
		{"gohawk", "-enable-checks"},
	} {
		if _, _, err := requestedChecks(arguments, metadata); err == nil {
			t.Errorf("arguments %v unexpectedly succeeded", arguments)
		}
	}
}

func TestCheckSelectionProfiles(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	testContext := "testlifecycle/context-root"
	nilContext := "contextpolicy/nil-context"

	t.Run("check alone selects only that check", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{testContext: true}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "./..."}, analyzers, groups, metadata, checkOwners(requested.enabled, metadata), false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(selection.arguments, " "), "-testlifecycle=true") || selection.normallySelected["testlifecycle"] {
			t.Fatalf("selection = %+v", selection)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if disabled[testContext] || !disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})

	t.Run("check adds to selected analyzer defaults", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{testContext: true}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection(
			[]string{"gohawk", "-enable=contextpolicy", "./..."},
			analyzers,
			groups,
			metadata,
			checkOwners(requested.enabled, metadata),
			false,
		)
		if err != nil {
			t.Fatal(err)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if disabled[testContext] || disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})

	t.Run("enable all includes every tier and disable wins", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{}, disabled: map[string]bool{testContext: true}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "-enable-all", "./..."}, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if !disabled[testContext] || disabled[nilContext] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})
}

func TestCheckSelectionTiers(t *testing.T) {
	analyzers := gohawk.Analyzers()
	groups := gohawk.AnalyzerGroups()
	metadata := gohawk.AnalyzerMetadata()
	nilContext := "contextpolicy/nil-context"

	t.Run("tier ceiling admits extended checks", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "-tier=extended", "./..."}, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if !selection.normallySelected["apishape"] || selection.normallySelected["taintpolicy"] {
			t.Fatalf("selected analyzers = %v", selection.normallySelected)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if disabled["apishape/parameter-count"] || disabled[nilContext] || !disabled["goroutineownership/detached"] {
			t.Fatalf("disabled checks = %v", disabled)
		}
	})

	t.Run("naming an analyzer admits extended but not experimental checks", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{}, disabled: map[string]bool{}}
		arguments := []string{"gohawk", "-enable=goroutineownership,lockorder", "./..."}
		selection, err := withAnalyzerCheckSelection(arguments, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if disabled["lockorder/contradictory-order"] || !disabled["goroutineownership/detached"] || disabled["goroutineownership/unjoined"] {
			t.Fatalf("disabled checks = %v", disabled)
		}

		arguments = []string{"gohawk", "-tier=experimental", "-enable=goroutineownership", "./..."}
		selection, err = withAnalyzerCheckSelection(arguments, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if disabled := effectiveDisabledChecks(metadata, selection, requested); disabled["goroutineownership/detached"] {
			t.Fatalf("experimental ceiling did not admit detached: %v", disabled)
		}
	})

	t.Run("unknown tier is rejected", func(t *testing.T) {
		if _, err := withAnalyzerCheckSelection([]string{"gohawk", "-tier=stable", "./..."}, analyzers, groups, metadata, nil, false); err == nil {
			t.Fatal("expected an error for an unknown tier")
		}
	})
}
