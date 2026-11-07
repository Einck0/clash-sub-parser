# Data and contracts

No schema migration. ConsistentFacts remains source of runtime snapshot. Projection produces a copied ResolvedPolicySnapshot containing actual rendered node IDs; stored nodes/groups/settings are not rewritten. ManifestExcludedNode carries stable node_id, safe code/reason. Artifact.ExcludedNodes exposes those exclusions to management; HTTP clients receive aggregate X-Export-Excluded-Count. Included manifests and node counts are computed after projection.

Successful preview creates immutable draft Publication with actual content/digests, real persisted ID and payload references; Publish validates and activates that same content. Existing active publication bytes remain unchanged. Preview mode supports strict/compatible, default strict Mihomo and compatible other targets. Direct/live fixed behavior is strict Mihomo/compatible other targets, with chains still rejected on non-Mihomo.
