package recon

// MaxGlobalPPS is the hard ceiling on packets-per-second for any single
// recon scan, regardless of allowlist or directive configuration. ADR
// 003 D11 defense-in-depth: a misconfigured allowlist or a malicious
// SaaS directive cannot push the agent above this rate. Tune via agent
// release; not customer-configurable.
const MaxGlobalPPS = 1000
