"""MQTT adapter: subscribes to telemetry and publishes scores."""

from __future__ import annotations

import json
import logging
from collections.abc import Callable
from urllib.parse import urlsplit

import paho.mqtt.client as mqtt

from ..domain import Reading

log = logging.getLogger(__name__)

TELEMETRY_SUBSCRIPTION = "plantorq/+/+/telemetry"


def parse_broker(url: str) -> tuple[str, int]:
    """Split a broker URL such as ``tcp://mosquitto:1883`` into host and port.

    Uses the same MQTT_BROKER format as the Go services, so every service is
    configured the same way.
    """
    parsed = urlsplit(url if "://" in url else f"tcp://{url}")
    if parsed.scheme not in ("tcp", "mqtt") or not parsed.hostname:
        raise ValueError(f"unsupported MQTT broker URL {url!r} (expected tcp://host:port)")
    return parsed.hostname, parsed.port or 1883


def parse_topic(topic: str) -> tuple[str, str] | None:
    """Return (tenant, machine) for a telemetry topic, else None."""
    parts = topic.split("/")
    if len(parts) != 4 or parts[0] != "plantorq" or parts[3] != "telemetry":
        return None
    return parts[1], parts[2]


class MqttBus:
    def __init__(self, host: str, port: int, client_id: str, username: str = "", password: str = "") -> None:
        self._client = mqtt.Client(mqtt.CallbackAPIVersion.VERSION2, client_id=client_id)
        if username:
            self._client.username_pw_set(username, password or None)
        self._client.reconnect_delay_set(min_delay=1, max_delay=30)
        self._host, self._port = host, port
        self._handler: Callable[[Reading], None] | None = None
        self._client.on_connect = self._on_connect
        self._client.on_message = self._on_message

    def publish(self, reading: Reading) -> None:
        topic = f"plantorq/{reading.tenant}/{reading.machine}/analytics"
        self._client.publish(topic, json.dumps(reading.to_dict()), qos=1)

    def run(self, handler: Callable[[Reading], None]) -> None:
        self._handler = handler
        self._client.connect_async(self._host, self._port)
        self._client.loop_forever(retry_first_connection=True)

    def _on_connect(self, client, _userdata, _flags, reason_code, _properties) -> None:
        log.info("connected to MQTT (%s)", reason_code)
        client.subscribe(TELEMETRY_SUBSCRIPTION, qos=1)

    def _on_message(self, _client, _userdata, msg) -> None:
        route = parse_topic(msg.topic)
        if route is None or self._handler is None:
            return
        try:
            reading = Reading.from_dict(json.loads(msg.payload), *route)
        except (ValueError, TypeError) as exc:
            log.warning("dropping malformed message on %s: %s", msg.topic, exc)
            return
        self._handler(reading)
