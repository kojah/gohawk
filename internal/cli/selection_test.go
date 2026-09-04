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

	selected := strings.Join(selectArguments([]string{"gohawk", "-enable=channelsafety", "./..."}), " ")
	if !strings.Contains(selected, "-channelsafety=true") || strings.Contains(selected, "-enable=") {
		t.Fatalf("selected arguments = %s", selected)
	}
	help := []string{"gohawk", "help", "channelsafety"}
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
	for _, value := range []string{"-lockorder=true", "-syncmapatomicity=true"} {
		if !strings.Contains(joined, value) {
			t.Errorf("default arguments do not contain %q: %v", value, got)
		}
	}
	for _, value := range []string{"-oncepolicy=true", "-channelsafety=true", "-channelsafety=true", "-channelsafety=true"} {
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
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups=ownership,reliability", "./..."}), " ")
		for _, value := range []string{
			"-lockorder=true", "-channelsafety=true", "-channelsafety=true", "-borrowedstorage=true",
		} {
			if !strings.Contains(got, value) {
				t.Errorf("group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-oncepolicy=true", "-channelsafety=true", "-enable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("groups combine with individual selection and exclusion", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-groups", "ownership", "-enable=channelsafety", "-disable=channelsafety", "./..."}), " ")
		for _, value := range []string{"-cancellationownership=true", "-goroutineownership=true", "-channelsafety=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("combined arguments do not contain %q: %s", value, got)
			}
		}
		if strings.Contains(got, "-channelsafety=true") {
			t.Errorf("explicit exclusion did not remove channelsafety: %s", got)
		}
	})

	t.Run("disabled groups subtract from defaults and allow individual overrides", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-disable-groups=reliability", "-enable=oncepolicy", "./..."}), " ")
		for _, value := range []string{"-lockorder=true", "-channelsafety=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("disabled-group arguments do not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-concurrentcapture=true", "-channelsafety=true", "-disable-groups"} {
			if strings.Contains(got, value) {
				t.Errorf("disabled-group arguments unexpectedly contain %q: %s", value, got)
			}
		}
	})

	t.Run("disabled groups subtract from enable-all", func(t *testing.T) {
		got := strings.Join(selectArguments([]string{"gohawk", "-enable-all", "-disable-groups=reliability", "./..."}), " ")
		for _, value := range []string{"-channelsafety=true", "-oncepolicy=true"} {
			if !strings.Contains(got, value) {
				t.Errorf("enable-all exclusion does not contain %q: %s", value, got)
			}
		}
		for _, value := range []string{"-channelsafety=true", "-disable-groups"} {
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
			{"gohawk", "-enable-groups=reliability", "-disable-groups=reliability", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("arguments %v unexpectedly succeeded", arguments)
			}
		}
	})

	t.Run("invalid analyzer lists", func(t *testing.T) {
		for _, arguments := range [][]string{
			{"gohawk", "-enable=unknown", "./..."},
			{"gohawk", "-enable=channelsafety,channelsafety", "./..."},
			{"gohawk", "-disable=oncepolicy,oncepolicy", "./..."},
			{"gohawk", "-enable=channelsafety", "-disable=channelsafety", "./..."},
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
				"-enable=oncepolicy,lockorder",
				"-disable=oncepolicy,lockorder",
				"./...",
			},
			analyzers,
			groups,
			metadata,
			false,
		)
		if got, want := err.Error(), `analyzer "lockorder" cannot be both enabled and disabled`; got != want {
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
			{"gohawk", "-channelsafety", "./..."},
			{"gohawk", "-oncepolicy=false", "./..."},
		} {
			if _, err := withAnalyzerSelection(arguments, analyzers, groups, metadata, false); err == nil {
				t.Errorf("legacy arguments %v unexpectedly succeeded", arguments)
			}
		}
		if _, err := withAnalyzerSelection([]string{"gohawk", "-channelsafety=true", "./..."}, analyzers, groups, metadata, true); err != nil {
			t.Fatalf("internal analyzer selection failed: %v", err)
		}
	})
}

func TestRequestedDisabledChecks(t *testing.T) {
	metadata := gohawk.AnalyzerMetadata()

	disabled, remaining, err := requestedDisabledChecks([]string{
		"gohawk",
		"-disable-checks=lockorder/missing-release,lockorder/missing-release",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"lockorder/missing-release", "lockorder/missing-release"} {
		if !disabled[check] {
			t.Errorf("disabled checks do not contain %q: %v", check, disabled)
		}
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-disable-checks=unknown/check", "./..."},
		{"gohawk", "-disable-checks=lockorder/missing-release,lockorder/missing-release", "./..."},
		{"gohawk", "-disable-checks=lockorder/missing-release,", "./..."},
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
		"-enable-checks=lockorder/contradictory-order,lockorder/missing-release",
		"-disable-checks=lockorder/missing-release",
		"./...",
	}, metadata)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []string{"lockorder/contradictory-order", "lockorder/missing-release"} {
		if !requested.enabled[check] {
			t.Errorf("enabled checks do not contain %q: %v", check, requested.enabled)
		}
	}
	if !requested.disabled["lockorder/missing-release"] {
		t.Errorf("disabled checks = %v", requested.disabled)
	}
	if want := []string{"gohawk", "./..."}; !slices.Equal(remaining, want) {
		t.Fatalf("remaining arguments = %v, want %v", remaining, want)
	}

	for _, arguments := range [][]string{
		{"gohawk", "-enable-checks=unknown/check", "./..."},
		{"gohawk", "-enable-checks=lockorder/contradictory-order,lockorder/contradictory-order", "./..."},
		{"gohawk", "-enable-checks=lockorder/contradictory-order,", "./..."},
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
	testContext := "lockorder/contradictory-order"
	nilContext := "lockorder/missing-release"

	t.Run("check alone selects only that check", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{testContext: true}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "./..."}, analyzers, groups, metadata, checkOwners(requested.enabled, metadata), false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(strings.Join(selection.arguments, " "), "-channelsafety=true") || selection.normallySelected["channelsafety"] {
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
			[]string{"gohawk", "-enable=lockorder", "./..."},
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
	nilContext := "lockorder/missing-release"

	t.Run("tier ceiling admits extended checks", func(t *testing.T) {
		requested := checkSelection{enabled: map[string]bool{}, disabled: map[string]bool{}}
		selection, err := withAnalyzerCheckSelection([]string{"gohawk", "-tier=extended", "./..."}, analyzers, groups, metadata, nil, false)
		if err != nil {
			t.Fatal(err)
		}
		if !selection.normallySelected["channelsafety"] || selection.normallySelected["borrowedstorage"] {
			t.Fatalf("selected analyzers = %v", selection.normallySelected)
		}
		disabled := effectiveDisabledChecks(metadata, selection, requested)
		if disabled["lockorder/contradictory-order"] || disabled[nilContext] || !disabled["goroutineownership/detached"] {
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
