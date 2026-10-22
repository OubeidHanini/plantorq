"""Physical behaviour of the simulated machines.

Each machine type is a ``MachineProfile`` (Strategy pattern): the server only
calls ``step()`` and never needs to know how a CNC mill differs from a
compressor. Adding a machine type means adding one class.
"""

from __future__ import annotations

import math
import random
from dataclasses import dataclass, field
from typing import Protocol


@dataclass(frozen=True)
class Snapshot:
    """Engineering values of one machine at one instant."""

    temperature: float  # °C
    power_kw: float
    vibration: float  # mm/s RMS
    current: float  # A
    spindle_rpm: float
    running: bool


class MachineProfile(Protocol):
    def step(self, dt: float) -> Snapshot: ...


@dataclass
class CncMill:
    """A milling machine whose load follows a machining cycle.

    With ``overheat_cycle`` set, the spindle slowly heats past the 70 °C alert
    threshold and cools down again, so the demo shows alerts firing and
    resolving on their own.
    """

    base_temp: float = 52.0
    overheat_cycle: bool = False
    rng: random.Random = field(default_factory=random.Random)
    _t: float = 0.0

    def step(self, dt: float) -> Snapshot:
        self._t += dt
        load = 0.75 + 0.2 * math.sin(2 * math.pi * self._t / 40)
        temp = self.base_temp + 4 * load
        if self.overheat_cycle:
            # 4-minute cycle: ramp up for 3 minutes, then cool down for 1.
            phase = (self._t % 240) / 240
            temp += 22 * (phase / 0.75 if phase < 0.75 else (1 - phase) / 0.25)
        return Snapshot(
            temperature=temp + self.rng.gauss(0, 0.3),
            power_kw=22 * load + self.rng.gauss(0, 0.4),
            vibration=2.2 + 1.5 * load + self.rng.gauss(0, 0.15),
            current=38 * load + self.rng.gauss(0, 0.5),
            spindle_rpm=round(9000 * load),
            running=True,
        )


@dataclass
class WeldingRobot:
    rng: random.Random = field(default_factory=random.Random)
    _t: float = 0.0

    def step(self, dt: float) -> Snapshot:
        self._t += dt
        welding = (self._t % 12) < 8  # 8 s weld, 4 s repositioning
        return Snapshot(
            temperature=41 + (3 if welding else 0) + self.rng.gauss(0, 0.2),
            power_kw=(8.4 if welding else 1.2) + self.rng.gauss(0, 0.2),
            vibration=0.8 + self.rng.gauss(0, 0.05),
            current=(31 if welding else 4) + self.rng.gauss(0, 0.4),
            spindle_rpm=0,
            running=True,
        )


@dataclass
class AirCompressor:
    """A compressor with an intermittent bearing fault.

    Every ~90 s vibration spikes for a few seconds: below the fixed 7 mm/s
    threshold most of the time, but statistically abnormal, which is exactly
    what the analytics service is there to catch.
    """

    rng: random.Random = field(default_factory=random.Random)
    _t: float = 0.0

    def step(self, dt: float) -> Snapshot:
        self._t += dt
        fault = (self._t % 90) < 6
        return Snapshot(
            temperature=61 + 1.5 * math.sin(2 * math.pi * self._t / 120) + self.rng.gauss(0, 0.2),
            power_kw=31 + self.rng.gauss(0, 0.6),
            vibration=(6.2 if fault else 3.0) + self.rng.gauss(0, 0.12),
            current=54 + self.rng.gauss(0, 0.6),
            spindle_rpm=1480,
            running=True,
        )
