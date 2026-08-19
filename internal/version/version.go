// Package version holds the Kairo release version.
package version

import "os"

// Version is the Kairo release version. It is read from the KAIRO_VERSION
// environment variable, which the release image sets from the git tag. Local
// builds without the variable fall back to "dev".
var Version = read()

const envKey = "KAIRO_VERSION"

func read() string {
	if v := os.Getenv(envKey); v != "" {
		return v
	}
	return "dev"
}
