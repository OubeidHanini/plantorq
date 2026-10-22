"""Prometheus adapter for the analytics Instrumentation port."""

from __future__ import annotations

from prometheus_client import CollectorRegistry, Counter, Histogram, start_http_server

ANOMALY_THRESHOLD = 0.8


class PrometheusInstrumentation:
    def __init__(self, registry: CollectorRegistry | None = None) -> None:
        kwargs = {"registry": registry} if registry is not None else {}
        self.processed = Counter(
            "plantorq_analytics_readings_processed_total", "Readings received for scoring.", **kwargs
        )
        self.anomalies = Counter(
            "plantorq_analytics_anomalies_total", f"Scores at or above {ANOMALY_THRESHOLD}.", **kwargs
        )
        self.scores = Histogram(
            "plantorq_analytics_anomaly_score",
            "Distribution of published anomaly scores.",
            buckets=(0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0),
            **kwargs,
        )

    def reading_processed(self, score: float | None) -> None:
        self.processed.inc()
        if score is None:
            return
        self.scores.observe(score)
        if score >= ANOMALY_THRESHOLD:
            self.anomalies.inc()

    @staticmethod
    def serve(port: int) -> None:
        """Expose /metrics on a background thread."""
        start_http_server(port)
