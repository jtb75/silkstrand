// Package audit provides fire-and-forget audit event emission per ADR 005.
// Event types are string constants so new types can be added without
// migrations. The Writer interface decouples callers from the storage
// backend; PostgresWriter batches inserts, NoopWriter drops silently.
package audit

// Event types — the canonical list. Add new entries here and in the
// ADR 005 taxonomy table. Review additions at PR time to keep the
// namespace clean.
const (
	// Credentials
	EventCredentialFetch   = "credential.fetch"
	EventCredentialCreated = "credential.created"
	EventCredentialUpdated = "credential.updated"
	EventCredentialDeleted = "credential.deleted"
	EventCredentialMapped  = "credential.mapped"
	EventCredentialUnmapped = "credential.unmapped"
	EventCredentialTest    = "credential.test"

	// Scans
	EventScanDispatched        = "scan.dispatched"
	EventScanCompleted         = "scan.completed"
	EventScanFailed            = "scan.failed"
	EventScanDefCreated        = "scan_definition.created"
	EventScanDefUpdated        = "scan_definition.updated"
	EventScanDefDeleted        = "scan_definition.deleted"
	EventScanDefExecuted       = "scan_definition.executed"

	// Correlation rules
	EventRuleCreated = "rule.created"
	EventRuleUpdated = "rule.updated"
	EventRuleDeleted = "rule.deleted"
	EventRuleFired   = "rule.fired"

	// Agents
	EventAgentConnected    = "agent.connected"
	EventAgentDisconnected = "agent.disconnected"
	EventAgentUpgraded     = "agent.upgraded"
	EventAgentKeyRotated   = "agent.key_rotated"
	EventAgentDeleted      = "agent.deleted"
	EventAgentCreated      = "agent.created"

	// Collections
	EventCollectionCreated = "collection.created"
	EventCollectionUpdated = "collection.updated"
	EventCollectionDeleted = "collection.deleted"

	// Bundles
	EventBundleUploaded = "bundle.uploaded"
	EventBundleDeleted  = "bundle.deleted"

	// Profiles
	EventProfileCreated   = "profile.created"
	EventProfilePublished = "profile.published"
	EventProfileDeleted   = "profile.deleted"
)

// Actor types.
const (
	ActorUser   = "user"
	ActorAgent  = "agent"
	ActorSystem = "system"
)
