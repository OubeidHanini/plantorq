"""OPC UA server exposing simulated machines as typed variables.

Same machine profiles as the Modbus server, a different protocol adapter:
each machine becomes an object with one variable per measurement, addressed
as ``ns=<idx>;s=<machine>.<measurement>``.
"""

from __future__ import annotations

import asyncio
import logging
from dataclasses import fields

from asyncua import Server, ua

from .profiles import MachineProfile, Snapshot

log = logging.getLogger(__name__)

NAMESPACE_URI = "urn:plantorq:simulator"


def _variant_type(value: object) -> ua.VariantType:
    return ua.VariantType.Boolean if isinstance(value, bool) else ua.VariantType.Double


class OpcUaPlant:
    """Owns the OPC UA machines and keeps their variables up to date."""

    def __init__(self, machines: dict[str, MachineProfile], tick: float = 1.0) -> None:
        self._machines = machines
        self._tick = tick
        self.namespace_index: int | None = None
        self._variables: dict[str, dict[str, object]] = {}

    async def _build(self, server: Server) -> None:
        idx = await server.register_namespace(NAMESPACE_URI)
        self.namespace_index = idx
        for name, profile in self._machines.items():
            machine = await server.nodes.objects.add_object(ua.NodeId(name, idx), name)
            snapshot = profile.step(self._tick)
            self._variables[name] = {}
            for f in fields(Snapshot):
                value = getattr(snapshot, f.name)
                node_id = ua.NodeId(f"{name}.{f.name}", idx)
                var = await machine.add_variable(node_id, f.name, value, varianttype=_variant_type(value))
                self._variables[name][f.name] = var

    async def update_once(self) -> None:
        for name, profile in self._machines.items():
            snapshot = profile.step(self._tick)
            for field_name, var in self._variables[name].items():
                value = getattr(snapshot, field_name)
                await var.write_value(ua.Variant(value, _variant_type(value)))

    async def serve(self, host: str, port: int, ready: asyncio.Event | None = None) -> None:
        server = Server()
        await server.init()
        server.set_endpoint(f"opc.tcp://{host}:{port}/")
        server.set_server_name("Plantorq simulated plant")
        # Demo only: production servers expose Basic256Sha256 with certificates.
        server.set_security_policy([ua.SecurityPolicyType.NoSecurity])
        await self._build(server)
        async with server:
            log.info(
                "OPC UA: simulating %d machine(s) on opc.tcp://%s:%d (ns=%d)",
                len(self._machines),
                host,
                port,
                self.namespace_index,
            )
            if ready is not None:
                ready.set()
            while True:
                await asyncio.sleep(self._tick)
                await self.update_once()
