"""Anomaly detection strategies.

``AnomalyDetector`` is the contract; ``RollingZScore`` is the current
strategy. A model-based detector (e.g. IsolationForest) can replace it without
touching the application service.
"""

from __future__ import annotations

import math
from collections import deque
from typing import Protocol


class AnomalyDetector(Protocol):
    def score(self, value: float) -> float | None:
        """Feed one value; return an anomaly score in [0, 1], or None while warming up."""


class RollingZScore:
    """Scores how far a value sits from the recent mean, in standard deviations.

    Uses a fixed-size window with running sums, so each update is O(1) in time
    and memory regardless of how long the service runs. ``z_max`` standard
    deviations map to a score of 1.0.
    """

    def __init__(self, window: int = 120, warmup: int = 30, z_max: float = 4.0, min_std: float = 1e-3) -> None:
        if not 2 <= warmup <= window:
            raise ValueError("require 2 <= warmup <= window")
        self._values: deque[float] = deque(maxlen=window)
        self._sum = 0.0
        self._sum_sq = 0.0
        self._warmup = warmup
        self._z_max = z_max
        self._min_std = min_std

    def score(self, value: float) -> float | None:
        result = None
        n = len(self._values)
        if n >= self._warmup:
            mean = self._sum / n
            variance = max(0.0, self._sum_sq / n - mean * mean)
            std = max(math.sqrt(variance), self._min_std)
            result = min(1.0, abs(value - mean) / std / self._z_max)
        self._push(value)
        return result

    def _push(self, value: float) -> None:
        if len(self._values) == self._values.maxlen:
            old = self._values[0]
            self._sum -= old
            self._sum_sq -= old * old
        self._values.append(value)
        self._sum += value
        self._sum_sq += value * value
