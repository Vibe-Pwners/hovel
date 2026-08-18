from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Any
from urllib.parse import quote

from hovel_sdk.session import Session, SessionRef

if TYPE_CHECKING:
    from hovel_sdk.session import SessionRegistry


class ChainKV:
    def __init__(self, target: str, payload: Any = None) -> None:
        self._target = target
        self._available = isinstance(payload, dict)
        self._revision = int(payload.get("revision", 0)) if isinstance(payload, dict) else 0
        entries = payload.get("entries", {}) if isinstance(payload, dict) else {}
        self._entries = {str(key): str(value) for key, value in entries.items()} if isinstance(entries, dict) else {}
        self._operations: list[dict[str, str]] = []

    @property
    def available(self) -> bool:
        return self._available

    @property
    def revision(self) -> int:
        return self._revision

    def _expand(self, key: str) -> str:
        return key.replace("{target}", quote(self._target, safe=""))

    def get(self, key: str, default: str | None = None) -> str | None:
        return self._entries.get(self._expand(key), default)

    def exists(self, key: str) -> bool:
        return self._expand(key) in self._entries

    def set(self, key: str, value: str) -> None:
        if not self.available:
            raise RuntimeError("hovel: chain kv is not available in this runtime")
        key = self._expand(key)
        if not key.strip():
            raise ValueError("hovel: chain kv key is required")
        self._entries[key] = value
        self._operations.append({"operation": "set", "key": key, "value": value})

    def delete(self, key: str) -> None:
        if not self.available:
            raise RuntimeError("hovel: chain kv is not available in this runtime")
        key = self._expand(key)
        if not key.strip():
            raise ValueError("hovel: chain kv key is required")
        self._entries.pop(key, None)
        self._operations.append({"operation": "delete", "key": key})

    def to_rpc(self) -> dict[str, Any] | None:
        if not self.available or not self._operations:
            return None
        return {"baseRevision": self.revision, "operations": list(self._operations)}


@dataclass(frozen=True)
class AgentEntity:
    id: str = ""
    kind: str = ""
    display_name: str = ""
    agent: bool = False

    @classmethod
    def from_rpc(cls, value: Any) -> AgentEntity:
        if not isinstance(value, dict):
            return cls()
        return cls(
            id=str(value.get("id", "")),
            kind=str(value.get("kind", "")),
            display_name=str(value.get("displayName", "")),
            agent=bool(value.get("agent", False)),
        )


@dataclass(frozen=True)
class AgentContext:
    schema: str = ""
    entity: AgentEntity = field(default_factory=AgentEntity)
    operation: str = ""
    chain: str = ""
    plan_id: str = ""
    plan_hash: str = ""
    approval_state: str = ""
    phase: str = ""
    resources: tuple[str, ...] = ()

    @classmethod
    def from_rpc(cls, value: Any) -> AgentContext | None:
        if not isinstance(value, dict):
            return None
        resources = value.get("resources") or ()
        if not isinstance(resources, (list, tuple)):
            resources = ()
        return cls(
            schema=str(value.get("schema", "")),
            entity=AgentEntity.from_rpc(value.get("entity")),
            operation=str(value.get("operation", "")),
            chain=str(value.get("chain", "")),
            plan_id=str(value.get("planId", "")),
            plan_hash=str(value.get("planHash", "")),
            approval_state=str(value.get("approvalState", "")),
            phase=str(value.get("phase", "")),
            resources=tuple(str(item) for item in resources),
        )


@dataclass(frozen=True)
class Context:
    run_id: str
    module_id: str
    target: str
    inputs: dict[str, Any] = field(default_factory=dict)
    chain_config: dict[str, Any] = field(default_factory=dict)
    target_config: dict[str, Any] = field(default_factory=dict)
    agent: AgentContext | None = None
    log: logging.Logger = field(default_factory=lambda: logging.getLogger("hovel.module"))
    sessions: SessionRegistry | None = field(default=None, repr=False)
    chain_kv: ChainKV = field(default_factory=lambda: ChainKV(""), repr=False)

    def input(self, key: str, default: Any = None) -> Any:
        if key in self.inputs:
            return self.inputs[key]
        if key in self.target_config:
            return self.target_config[key]
        return self.chain_config.get(key, default)

    def resolve_input(self, config_key: str, kv_key: str, default: Any = None) -> tuple[Any, str, bool]:
        if config_key in self.inputs:
            return self.inputs[config_key], "input", True
        if config_key in self.target_config:
            return self.target_config[config_key], "target-config", True
        if config_key in self.chain_config:
            return self.chain_config[config_key], "chain-config", True
        if self.chain_kv.exists(kv_key):
            return self.chain_kv.get(kv_key), "chain-kv", True
        return default, "default", False

    async def open_session(
        self,
        session: Session,
        *,
        name: str = "",
        kind: str = "shell",
        transport: str = "stdio",
        capabilities: tuple[str, ...] = ("read", "write", "close"),
    ) -> SessionRef:
        if self.sessions is None:
            raise RuntimeError("session support is not available in this runtime")
        return await self.sessions.open(
            session,
            name=name,
            kind=kind,
            transport=transport,
            capabilities=capabilities,
        )
