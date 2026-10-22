"""Use case: turn raw readings into anomaly scores."""

from __future__ import annotations

import logging
from collections.abc import Callable, Iterable
from typing import Protocol

from ..domain import AnomalyDetector, Reading

log = logging.getLogger(__name__)

DetectorFactory = Callable[[], AnomalyDetector]


class ScorePublisher(Protocol):
    """Port: where computed scores go (MQTT in production, a list in tests)."""

    def publish(self, reading: Reading) -> None: ...


class Instrumentation(Protocol):
    """Port: operational signals (Prometheus in production)."""

    def reading_processed(self, score: float | None) -> None: ...


class NoInstrumentation:
    def reading_processed(self, score: float | None) -> None:
        pass


class AnalyticsService:
    """Keeps one detector per (tenant, machine, metric) and publishes the
    highest score of each reading as ``anomaly_score``.

    The service depends on a factory and a port, not on concrete classes
    (dependency inversion), so detectors and transports are swappable.
    """

    def __init__(
        self,
        watched: Iterable[str],
        detector_factory: DetectorFactory,
        publisher: ScorePublisher,
        instrumentation: Instrumentation | None = None,
    ) -> None:
        self._watched = tuple(watched)
        self._new_detector = detector_factory
        self._publisher = publisher
        self._instrumentation = instrumentation or NoInstrumentation()
        self._detectors: dict[tuple[str, str, str], AnomalyDetector] = {}

    def handle(self, reading: Reading) -> Reading | None:
        scores = []
        for metric in self._watched:
            if metric not in reading.values:
                continue
            key = (reading.tenant, reading.machine, metric)
            detector = self._detectors.get(key)
            if detector is None:
                detector = self._detectors[key] = self._new_detector()
            score = detector.score(reading.values[metric])
            if score is not None:
                scores.append(score)
        if not scores:
            self._instrumentation.reading_processed(None)
            return None

        result = Reading(
            tenant=reading.tenant,
            machine=reading.machine,
            ts=reading.ts,
            values={"anomaly_score": round(max(scores), 3)},
            source="analytics",
        )
        self._publisher.publish(result)
        self._instrumentation.reading_processed(max(scores))
        if max(scores) >= 0.8:
            log.info("anomaly on %s/%s: score %.2f", reading.tenant, reading.machine, max(scores))
        return result
