// prism/internal/version/version.go
package version

// Version is set by the linker via -ldflags "-X .../version.Version=...".
// Defaults to "dev" for local builds.
var Version = "dev"
