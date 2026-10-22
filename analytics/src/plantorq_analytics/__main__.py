"""Composition root: wire the detector strategy, the use case and MQTT."""

import logging
import os
import socket

from .application import AnalyticsService
from .domain import RollingZScore
from .infrastructure import MqttBus, PrometheusInstrumentation, parse_broker


def main() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s"
    )
    window = int(os.getenv("WINDOW", "120"))
    PrometheusInstrumentation.serve(int(os.getenv("METRICS_PORT", "9104")))
    host, port = parse_broker(os.getenv("MQTT_BROKER", "tcp://localhost:1883"))
    bus = MqttBus(
        host,
        port,
        f"analytics-{socket.gethostname()}",
        username=os.getenv("MQTT_USERNAME", ""),
        password=os.getenv("MQTT_PASSWORD", ""),
    )
    service = AnalyticsService(
        # Vibration is the leading indicator of bearing faults. Temperature is
        # load-driven and non-stationary, which makes z-scores noisy; it is
        # covered by threshold rules instead.
        watched=os.getenv("WATCHED_METRICS", "vibration").split(","),
        detector_factory=lambda: RollingZScore(window=window, warmup=min(30, window)),
        publisher=bus,
        instrumentation=PrometheusInstrumentation(),
    )
    bus.run(service.handle)


if __name__ == "__main__":
    main()
