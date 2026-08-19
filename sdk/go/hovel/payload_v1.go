package hovel

// PayloadSchemaV1 is the first versioned, portable payload-provider contract.
const PayloadSchemaV1 = "hovel.payload/v1"

// PayloadProviderDescriptor advertises only the independent payload operations
// a provider implements. Legacy PayloadProvider implementations remain valid.
type PayloadProviderDescriptor struct {
	SchemaVersion string                     `json:"schemaVersion"`
	ProviderID    string                     `json:"providerId"`
	Version       string                     `json:"version"`
	Operations    []PayloadProviderOperation `json:"operations"`
	Payloads      []PayloadVariant           `json:"payloads,omitempty"`
	Extensions    map[string]any             `json:"extensions,omitempty"`
}

type PayloadProviderOperation string

const (
	PayloadOperationResolve         PayloadProviderOperation = "resolve"
	PayloadOperationGenerate        PayloadProviderOperation = "generate"
	PayloadOperationReadArtifact    PayloadProviderOperation = "read-artifact"
	PayloadOperationPrepareListener PayloadProviderOperation = "prepare-listener"
	PayloadOperationConnect         PayloadProviderOperation = "connect"
	PayloadOperationInspect         PayloadProviderOperation = "inspect"
	PayloadOperationCleanup         PayloadProviderOperation = "cleanup"
	PayloadOperationCommands        PayloadProviderOperation = "commands"
)

// PayloadVariant is a provider-advertised buildable payload with an exact load
// contract. Extensions must use provider-owned, namespaced keys.
type PayloadVariant struct {
	ID           string              `json:"id"`
	Name         string              `json:"name"`
	Version      string              `json:"version"`
	Kind         PayloadKind         `json:"kind"`
	Format       string              `json:"format"`
	Target       PayloadTarget       `json:"target"`
	Load         PayloadLoadContract `json:"load"`
	Capabilities []string            `json:"capabilities,omitempty"`
	Extensions   map[string]any      `json:"extensions,omitempty"`
}

type PayloadTarget struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	ABI        string `json:"abi,omitempty"`
	Endianness string `json:"endianness,omitempty"`
	MinimumOS  string `json:"minimumOS,omitempty"`
}

type PayloadLoadContract struct {
	ExecutionModel string   `json:"executionModel"`
	EntryContract  string   `json:"entryContract,omitempty"`
	Relocation     string   `json:"relocation,omitempty"`
	Dependencies   []string `json:"dependencies,omitempty"`
}

// PayloadContent is a closed content union. Exactly one of Inline, Artifact,
// or Stream must be present in a valid hovel.payload/v1 artifact.
type PayloadContent struct {
	Inline   *PayloadInlineContent   `json:"inline,omitempty"`
	Artifact *PayloadArtifactContent `json:"artifact,omitempty"`
	Stream   *PayloadStreamContent   `json:"stream,omitempty"`
}

type PayloadInlineContent struct {
	Encoding string `json:"encoding"`
	Data     string `json:"data"`
}

type PayloadArtifactContent struct {
	ArtifactID string `json:"id"`
}

type PayloadStreamContent struct {
	Handle string `json:"handle"`
}

type PayloadArtifactV1 struct {
	SchemaVersion string         `json:"schemaVersion"`
	Name          string         `json:"name"`
	Role          string         `json:"role"`
	Variant       PayloadVariant `json:"variant"`
	MediaType     string         `json:"mediaType"`
	Size          int64          `json:"size"`
	SHA256        string         `json:"sha256"`
	Content       PayloadContent `json:"content"`
	Extensions    map[string]any `json:"extensions,omitempty"`
}

// Optional payload-provider surfaces. Providers implement only the operations
// they advertise through PayloadProviderDescriptor.
type PayloadDescriber interface {
	DescribePayloads() (PayloadProviderDescriptor, error)
}

type PayloadResolver interface {
	ResolvePayloadV1(PayloadQuery) (PayloadVariant, error)
}

type PayloadGenerator interface {
	GeneratePayloadV1(GeneratePayloadRequest) (PayloadArtifactV1, error)
}

type PayloadArtifactReader interface {
	ReadPayloadArtifact(ReadPayloadChunkRequest) (PayloadChunk, error)
}

type PayloadListenerPreparer interface {
	PreparePayloadListener(PrepareListenerRequest) (ListenerRef, error)
}

type PayloadConnector interface {
	ConnectPayload(ConnectSessionRequest) (SessionRef, error)
}

type PayloadInspector interface {
	InspectPayload(ConnectSessionRequest) (InstalledPayloadDescriptor, error)
}

type PayloadCleanupProvider interface {
	CleanupInstalledPayload(CleanupPayloadRequest) (CleanupResult, error)
}
