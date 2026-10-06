package goroutineownership

import "github.com/kojah/gohawk/internal/engine/enumtext"

// queryPhase names the closed discovery and lifetime requests whose cutoff
// evidence is attributed to a spawn. Labels are rendered only into trace details.
type queryPhase uint8

const (
	_ queryPhase = iota
	queryCompletionDiscovery
	queryRelayDiscovery
	queryOwnerDiscovery
	queryPipePeerDiscovery
	queryFactoryOrigin
	querySignalCensus
	queryRelayDependency
	queryCallerLifetime
	queryPreSpawnCensus
)

var queryPhaseLabels = [...]string{
	0:                        "",
	queryCompletionDiscovery: "completion-discovery",
	queryRelayDiscovery:      "relay-discovery",
	queryOwnerDiscovery:      "owner-discovery",
	queryPipePeerDiscovery:   "pipe-peer-discovery",
	queryFactoryOrigin:       "factory-origin",
	querySignalCensus:        "signal-census",
	queryRelayDependency:     "relay-dependency",
	queryCallerLifetime:      "caller-lifetime",
	queryPreSpawnCensus:      "pre-spawn-census",
}

func (phase queryPhase) String() string { return enumtext.Name(phase, queryPhaseLabels[:]) }
