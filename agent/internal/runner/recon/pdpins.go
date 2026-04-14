package recon

// PDTool is one ProjectDiscovery binary tracked for runtime install
// (ADR 003 R1a). Pins are populated in PR #6 (infra/seed-pd-binaries)
// once the silkstrand-runtimes bucket has objects.
type PDTool struct {
	Name    string                       // "naabu" | "httpx" | "nuclei"
	Version string                       // upstream semver
	SHA256  map[string]string            // platform key (e.g. "linux-amd64") → hex sha256
}

// pdTools is the compile-time pin table. Empty until PR #6.
//
// Format note (PR #6): keys in SHA256 are "<os>-<arch>" matching
// runtime.GOOS + "-" + runtime.GOARCH; values are lowercase hex.
var pdTools = []PDTool{
	{
		Name:    "naabu",
		Version: "",
		SHA256:  map[string]string{},
	},
	{
		Name:    "httpx",
		Version: "",
		SHA256:  map[string]string{},
	},
	{
		Name:    "nuclei",
		Version: "",
		SHA256:  map[string]string{},
	},
}

// nucleiTemplatesPin pins the SilkStrand-curated template tarball.
// Version is a SilkStrand semver, not upstream. Empty until PR #6.
var nucleiTemplatesPin = struct {
	Version string // e.g. "0.1.0"
	SHA256  string // hex of the .tar.gz
}{}
