import random

import pytest
from prometheus_client import CollectorRegistry

from plantorq_analytics.application import AnalyticsService
from plantorq_analytics.domain import Reading, RollingZScore
from plantorq_analytics.infrastructure import PrometheusInstrumentation, parse_broker, parse_topic


class ListPublisher:
    def __init__(self):
        self.published = []

    def publish(self, reading):
        self.published.append(reading)


def test_detector_warms_up_then_flags_outliers():
    rng = random.Random(42)
    d = RollingZScore(window=60, warmup=20)
    scores = [d.score(3.0 + rng.gauss(0, 0.1)) for _ in range(60)]
    assert scores[:20] == [None] * 20
    # Gaussian noise can reach ~3.5 sigma over 40 samples, but must never saturate.
    assert max(s for s in scores[20:] if s is not None) < 1.0
    assert d.score(6.5) == 1.0  # a spike of ~35 sigma saturates the score


def test_detector_window_forgets_old_values():
    d = RollingZScore(window=10, warmup=5)
    for _ in range(10):
        d.score(100.0)
    for _ in range(10):
        d.score(0.0)  # fully replaces the window
    assert d.score(0.0) == 0.0


def test_detector_validates_parameters():
    with pytest.raises(ValueError):
        RollingZScore(window=5, warmup=10)


def test_service_keeps_one_detector_per_machine_and_publishes_max_score():
    pub = ListPublisher()
    svc = AnalyticsService(["vibration"], lambda: RollingZScore(window=10, warmup=3), pub)
    for i in range(5):
        svc.handle(Reading("acme", "comp-c2", i, {"vibration": 3.0 + 0.01 * i}))
        svc.handle(Reading("acme", "cnc-01", i, {"vibration": 50.0}))
    out = svc.handle(Reading("acme", "comp-c2", 99, {"vibration": 9.0}))

    assert out is not None and out.values["anomaly_score"] == 1.0
    assert out.source == "analytics" and out.machine == "comp-c2"
    # cnc-01's values must not have polluted comp-c2's baseline (or vice versa)
    cnc = [r for r in pub.published if r.machine == "cnc-01"]
    assert all(r.values["anomaly_score"] == 0.0 for r in cnc)


def test_service_ignores_readings_without_watched_metrics():
    pub = ListPublisher()
    svc = AnalyticsService(["vibration"], RollingZScore, pub)
    assert svc.handle(Reading("acme", "m", 1, {"temperature": 50})) is None
    assert pub.published == []


def test_reading_takes_routing_from_topic_not_payload():
    r = Reading.from_dict({"tenant": "evil", "ts": 5, "values": {"x": 1, "bad": "nan?"}}, "acme", "cnc-01")
    assert (r.tenant, r.machine, r.values) == ("acme", "cnc-01", {"x": 1.0})


@pytest.mark.parametrize(
    "topic,expected",
    [
        ("plantorq/acme/cnc-01/telemetry", ("acme", "cnc-01")),
        ("plantorq/acme/cnc-01/analytics", None),
        ("plantorq/acme/alerts", None),
    ],
)
def test_parse_topic(topic, expected):
    assert parse_topic(topic) == expected


def test_instrumentation_counts_readings_and_anomalies():
    registry = CollectorRegistry()
    inst = PrometheusInstrumentation(registry)
    svc = AnalyticsService(["vibration"], lambda: RollingZScore(window=10, warmup=3), ListPublisher(), inst)
    for i in range(4):
        svc.handle(Reading("acme", "comp-c2", i, {"vibration": 3.0 + 0.01 * i}))
    svc.handle(Reading("acme", "comp-c2", 9, {"vibration": 9.0}))  # spike

    def value(name):
        return registry.get_sample_value(name)

    assert value("plantorq_analytics_readings_processed_total") == 5
    assert value("plantorq_analytics_anomalies_total") == 1
    assert value("plantorq_analytics_anomaly_score_count") == 2  # 3 warm-up readings publish no score


@pytest.mark.parametrize(
    "url,expected",
    [
        ("tcp://mosquitto:1883", ("mosquitto", 1883)),
        ("tcp://broker.local", ("broker.local", 1883)),
        ("mosquitto:1884", ("mosquitto", 1884)),
    ],
)
def test_parse_broker(url, expected):
    assert parse_broker(url) == expected


def test_parse_broker_rejects_other_schemes():
    with pytest.raises(ValueError):
        parse_broker("http://mosquitto:1883")
