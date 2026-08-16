package main

// version is set at build time via -ldflags "-X main.version=..."; dev builds
// fall back to the literal below. Bump it when the release changes.
var version = "0.1.0"
