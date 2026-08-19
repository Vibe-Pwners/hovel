from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Protocol

PAYLOAD_SCHEMA_V1 = "hovel.payload/v1"
PAYLOAD_RPC_PREFIX = "payload."


@dataclass(frozen=True)
class PayloadTarget:
    os: str
    arch: str
    abi: str = ""
    endianness: str = ""
    minimum_os: str = ""

    def to_rpc(self) -> dict[str, Any]:
        return {
            "os": self.os,
            "arch": self.arch,
            **{
                key: value
                for key, value in (("abi", self.abi), ("endianness", self.endianness), ("minimumOs", self.minimum_os))
                if value
            },
        }


@dataclass(frozen=True)
class PayloadLoadContract:
    execution_model: str
    entry_contract: str = ""
    relocation: str = ""
    dependencies: tuple[str, ...] = ()

    def to_rpc(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "executionModel": self.execution_model,
            **{
                key: value
                for key, value in (("entryContract", self.entry_contract), ("relocation", self.relocation))
                if value
            },
        }
        if self.dependencies:
            result["dependencies"] = list(self.dependencies)
        return result


@dataclass(frozen=True)
class PayloadVariant:
    id: str
    name: str
    version: str
    kind: str
    format: str
    target: PayloadTarget
    load: PayloadLoadContract
    capabilities: tuple[str, ...] = ()
    extensions: dict[str, Any] = field(default_factory=dict)

    def to_rpc(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "id": self.id,
            "name": self.name,
            "version": self.version,
            "kind": self.kind,
            "format": self.format,
            "target": self.target.to_rpc(),
            "load": self.load.to_rpc(),
        }
        if self.capabilities:
            result["capabilities"] = list(self.capabilities)
        if self.extensions:
            result["extensions"] = dict(self.extensions)
        return result


@dataclass(frozen=True)
class PayloadProviderDescriptor:
    provider_id: str
    version: str
    operations: tuple[str, ...]
    payloads: tuple[PayloadVariant, ...] = ()
    extensions: dict[str, Any] = field(default_factory=dict)
    schema_version: str = PAYLOAD_SCHEMA_V1

    def to_rpc(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "schemaVersion": self.schema_version,
            "providerId": self.provider_id,
            "version": self.version,
            "operations": list(self.operations),
        }
        if self.payloads:
            result["payloads"] = [payload.to_rpc() for payload in self.payloads]
        if self.extensions:
            result["extensions"] = dict(self.extensions)
        return result


@dataclass(frozen=True)
class PayloadContent:
    inline_encoding: str = ""
    inline_data: str = ""
    artifact_id: str = ""
    stream_handle: str = ""

    def to_rpc(self) -> dict[str, Any]:
        choices = [bool(self.inline_data), bool(self.artifact_id), bool(self.stream_handle)]
        if sum(choices) != 1:
            raise ValueError("payload content requires exactly one inline, artifact, or stream source")
        if self.inline_data:
            return {"inline": {"encoding": self.inline_encoding, "data": self.inline_data}}
        if self.artifact_id:
            return {"artifact": {"id": self.artifact_id}}
        return {"stream": {"handle": self.stream_handle}}


@dataclass(frozen=True)
class PayloadArtifactV1:
    name: str
    role: str
    variant: PayloadVariant
    media_type: str
    size: int
    sha256: str
    content: PayloadContent
    extensions: dict[str, Any] = field(default_factory=dict)
    schema_version: str = PAYLOAD_SCHEMA_V1

    def to_rpc(self) -> dict[str, Any]:
        result: dict[str, Any] = {
            "schemaVersion": self.schema_version,
            "name": self.name,
            "role": self.role,
            "variant": self.variant.to_rpc(),
            "mediaType": self.media_type,
            "size": self.size,
            "sha256": self.sha256,
            "content": self.content.to_rpc(),
        }
        if self.extensions:
            result["extensions"] = dict(self.extensions)
        return result


class PayloadDescriber(Protocol):
    def describe_payloads(self) -> PayloadProviderDescriptor: ...


class PayloadResolver(Protocol):
    def resolve_payload_v1(self, query: dict[str, Any]) -> PayloadVariant: ...


class PayloadGenerator(Protocol):
    def generate_payload_v1(self, request: dict[str, Any]) -> PayloadArtifactV1: ...


class PayloadArtifactReader(Protocol):
    def read_payload_artifact(self, request: dict[str, Any]) -> dict[str, Any]: ...


class PayloadListenerPreparer(Protocol):
    def prepare_payload_listener(self, request: dict[str, Any]) -> dict[str, Any]: ...


class PayloadConnector(Protocol):
    def connect_payload(self, request: dict[str, Any]) -> dict[str, Any]: ...


class PayloadInspector(Protocol):
    def inspect_payload(self, request: dict[str, Any]) -> dict[str, Any]: ...


class PayloadCleanupProvider(Protocol):
    def cleanup_installed_payload(self, request: dict[str, Any]) -> dict[str, Any]: ...
