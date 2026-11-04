package main

// Boundary regressions qualify the masking implementation. A candidate with
// this gate enabled must pass native acceptance before entering the feature PR.
var teamSecretMaskingReady = func() bool { return true }
