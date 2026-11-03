package main

// Layer 6 deliberately keeps credentials unavailable until layer 7 owns every
// outbound, streaming, persistence and history masking boundary.
func teamSecretMaskingReady() bool { return false }
