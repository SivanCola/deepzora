package main

// Layer 6 deliberately keeps credentials unavailable until layer 7 owns every
// outbound, streaming, persistence and history masking boundary.
var teamSecretMaskingReady = func() bool { return false }
