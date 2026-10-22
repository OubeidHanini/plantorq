"""Domain types mirroring the Go ``telemetry.Reading`` wire format."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class Reading:
    tenant: str
    machine: str
    ts: int  # Unix milliseconds
    values: dict[str, float] = field(default_factory=dict)
    source: str = "edge"

    @classmethod
    def from_dict(cls, data: dict, tenant: str, machine: str) -> Reading:
        """Build a reading; tenant and machine come from the topic, not the payload."""
        values = {k: float(v) for k, v in (data.get("values") or {}).items() if isinstance(v, (int, float))}
        return cls(
            tenant=tenant, machine=machine, ts=int(data.get("ts", 0)), values=values, source=data.get("source", "edge")
        )

    def to_dict(self) -> dict:
        return {
            "tenant": self.tenant,
            "machine": self.machine,
            "ts": self.ts,
            "values": self.values,
            "source": self.source,
        }
