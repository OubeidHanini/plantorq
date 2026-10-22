"""Encoding of engineering values into 16-bit holding registers.

This is the device side of the contract the edge agent decodes with
``value = raw * scale`` (see deploy/config/agent.yaml, profile ``standard``).
"""

from __future__ import annotations

from dataclasses import dataclass

from .profiles import Snapshot


@dataclass(frozen=True)
class RegisterSpec:
    address: int
    field: str
    scale: float


REGISTER_MAP: tuple[RegisterSpec, ...] = (
    RegisterSpec(0, "temperature", 0.1),
    RegisterSpec(1, "power_kw", 0.01),
    RegisterSpec(2, "vibration", 0.01),
    RegisterSpec(3, "current", 0.1),
    RegisterSpec(4, "spindle_rpm", 1),
    RegisterSpec(5, "running", 1),
)


def encode(snapshot: Snapshot) -> list[int]:
    """Return register values ordered by address, clamped to uint16."""
    words = []
    for spec in REGISTER_MAP:
        value = float(getattr(snapshot, spec.field))
        words.append(max(0, min(0xFFFF, round(value / spec.scale))))
    return words
