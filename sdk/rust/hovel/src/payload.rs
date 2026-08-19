use crate::json::Value;

pub const PAYLOAD_SCHEMA_V1: &str = "hovel.payload/v1";
pub(crate) const PAYLOAD_RPC_DESCRIBE_METHOD: &str = "payload.describe";
pub(crate) const PAYLOAD_RPC_RESOLVE_METHOD: &str = "payload.resolve";
pub(crate) const PAYLOAD_RPC_GENERATE_METHOD: &str = "payload.generate";
pub(crate) const PAYLOAD_RPC_ARTIFACT_READ_METHOD: &str = "payload.artifact.read";
pub(crate) const PAYLOAD_RPC_LISTENER_PREPARE_METHOD: &str = "payload.listener.prepare";
pub(crate) const PAYLOAD_RPC_CONNECT_METHOD: &str = "payload.connect";
pub(crate) const PAYLOAD_RPC_INSPECT_METHOD: &str = "payload.inspect";
pub(crate) const PAYLOAD_RPC_CLEANUP_METHOD: &str = "payload.cleanup";

#[derive(Clone, Debug, Default, PartialEq)]
pub struct PayloadTarget {
    pub os: String,
    pub arch: String,
    pub abi: String,
    pub endianness: String,
    pub minimum_os: String,
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct PayloadLoadContract {
    pub execution_model: String,
    pub entry_contract: String,
    pub relocation: String,
    pub dependencies: Vec<String>,
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct PayloadVariant {
    pub id: String,
    pub name: String,
    pub version: String,
    pub kind: String,
    pub format: String,
    pub target: PayloadTarget,
    pub load: PayloadLoadContract,
    pub capabilities: Vec<String>,
    pub extensions: Vec<(String, Value)>,
}

#[derive(Clone, Debug, Default, PartialEq)]
pub struct PayloadProviderDescriptor {
    pub schema_version: String,
    pub provider_id: String,
    pub version: String,
    pub operations: Vec<String>,
    pub payloads: Vec<PayloadVariant>,
    pub extensions: Vec<(String, Value)>,
}

#[derive(Clone, Debug, PartialEq)]
pub enum PayloadContent {
    Inline { encoding: String, data: String },
    Artifact { artifact_id: String },
    Stream { handle: String },
}

#[derive(Clone, Debug, PartialEq)]
pub struct PayloadArtifactV1 {
    pub schema_version: String,
    pub name: String,
    pub role: String,
    pub variant: PayloadVariant,
    pub media_type: String,
    pub size: u64,
    pub sha256: String,
    pub content: PayloadContent,
    pub extensions: Vec<(String, Value)>,
}

impl PayloadTarget {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            ("os", Value::Str(self.os.clone())),
            ("arch", Value::Str(self.arch.clone())),
            ("abi", Value::Str(self.abi.clone())),
            ("endianness", Value::Str(self.endianness.clone())),
            ("minimumOs", Value::Str(self.minimum_os.clone())),
        ])
    }
}

impl PayloadLoadContract {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            ("executionModel", Value::Str(self.execution_model.clone())),
            ("entryContract", Value::Str(self.entry_contract.clone())),
            ("relocation", Value::Str(self.relocation.clone())),
            (
                "dependencies",
                Value::Array(self.dependencies.iter().cloned().map(Value::Str).collect()),
            ),
        ])
    }
}

impl PayloadVariant {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            ("id", Value::Str(self.id.clone())),
            ("name", Value::Str(self.name.clone())),
            ("version", Value::Str(self.version.clone())),
            ("kind", Value::Str(self.kind.clone())),
            ("format", Value::Str(self.format.clone())),
            ("target", self.target.to_value()),
            ("load", self.load.to_value()),
            (
                "capabilities",
                Value::Array(self.capabilities.iter().cloned().map(Value::Str).collect()),
            ),
            ("extensions", Value::Object(self.extensions.clone())),
        ])
    }
}

impl PayloadProviderDescriptor {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            ("schemaVersion", Value::Str(self.schema_version.clone())),
            ("providerId", Value::Str(self.provider_id.clone())),
            ("version", Value::Str(self.version.clone())),
            (
                "operations",
                Value::Array(self.operations.iter().cloned().map(Value::Str).collect()),
            ),
            (
                "payloads",
                Value::Array(self.payloads.iter().map(PayloadVariant::to_value).collect()),
            ),
            ("extensions", Value::Object(self.extensions.clone())),
        ])
    }
}

impl PayloadContent {
    pub(crate) fn to_value(&self) -> Value {
        match self {
            Self::Inline { encoding, data } => Value::object(vec![(
                "inline",
                Value::object(vec![
                    ("encoding", Value::Str(encoding.clone())),
                    ("data", Value::Str(data.clone())),
                ]),
            )]),
            Self::Artifact { artifact_id } => Value::object(vec![(
                "artifact",
                Value::object(vec![("id", Value::Str(artifact_id.clone()))]),
            )]),
            Self::Stream { handle } => Value::object(vec![(
                "stream",
                Value::object(vec![("handle", Value::Str(handle.clone()))]),
            )]),
        }
    }
}

impl PayloadArtifactV1 {
    pub(crate) fn to_value(&self) -> Value {
        Value::object(vec![
            ("schemaVersion", Value::Str(self.schema_version.clone())),
            ("name", Value::Str(self.name.clone())),
            ("role", Value::Str(self.role.clone())),
            ("variant", self.variant.to_value()),
            ("mediaType", Value::Str(self.media_type.clone())),
            ("size", Value::from(self.size as i64)),
            ("sha256", Value::Str(self.sha256.clone())),
            ("content", self.content.to_value()),
            ("extensions", Value::Object(self.extensions.clone())),
        ])
    }
}
