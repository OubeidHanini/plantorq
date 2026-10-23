"""Integration test: a real OPC UA client reads the simulator over TCP."""

import asyncio
import random
import socket

from asyncua import Client

from plantorq_simulator.opcua_server import OpcUaPlant
from plantorq_simulator.profiles import WeldingRobot


def free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


async def read_robot(port: int) -> dict:
    plant = OpcUaPlant({"robot-07": WeldingRobot(rng=random.Random(1))}, tick=0.05)
    ready = asyncio.Event()
    server = asyncio.create_task(plant.serve("127.0.0.1", port, ready))
    try:
        await asyncio.wait_for(ready.wait(), timeout=15)
        ns = plant.namespace_index
        async with Client(f"opc.tcp://127.0.0.1:{port}/") as client:
            # Same node ids as deploy/config/agent.yaml uses for the Go agent.
            names = ["temperature", "power_kw", "vibration", "current", "running"]
            nodes = [client.get_node(f"ns={ns};s=robot-07.{n}") for n in names]
            first = dict(zip(names, await client.read_values(nodes), strict=True))
            await asyncio.sleep(0.3)
            second = await nodes[0].read_value()
            return {"ns": ns, "first": first, "temperature_later": second}
    finally:
        server.cancel()


def test_robot_variables_are_readable_and_typed():
    result = asyncio.run(read_robot(free_port()))
    values = result["first"]
    assert result["ns"] == 2, "agent config hard-codes ns=2"
    assert 35 < values["temperature"] < 50
    assert values["power_kw"] > 0
    assert isinstance(values["running"], bool)


def test_values_keep_updating():
    result = asyncio.run(read_robot(free_port()))
    assert result["temperature_later"] != result["first"]["temperature"]
