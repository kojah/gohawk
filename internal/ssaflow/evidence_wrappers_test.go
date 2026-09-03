package ssaflow

// Test-only conveniences over the memoized LocalEvidence proof methods.
// Production callers share one LocalEvidence per analyzer function, so these
// wrappers live with the tests instead of the package API.

func ProveOwnershipTransfer(request OwnershipTransferRequest) OwnershipTransferProof {
	var evidence LocalEvidence
	return evidence.OwnershipTransfer(request)
}
