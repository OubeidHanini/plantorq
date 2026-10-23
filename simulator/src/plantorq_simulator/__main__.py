"""Composition root: choose which machines exist and on which protocol."""

import asyncio
import logging
import os
import threading

from .opcua_server import OpcUaPlant
from .profiles import AirCompressor, CncMill, WeldingRobot
from .server import PlantSimulator


def main() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s"
    )
    host = os.getenv("BIND_HOST", "0.0.0.0")

    # The welding robot speaks OPC UA, the rest of the floor Modbus TCP, as in
    # a real plant where generations of equipment coexist.
    opcua_plant = OpcUaPlant({"robot-07": WeldingRobot()})
    threading.Thread(
        target=lambda: asyncio.run(opcua_plant.serve(host, int(os.getenv("OPCUA_PORT", "4840")))),
        name="opcua",
        daemon=True,
    ).start()

    # Unit 5 (press-p04) is deliberately absent: the agent polls it and the
    # dashboard shows how an unreachable machine is handled.
    modbus_machines = {
        1: ("cnc-01", CncMill()),
        2: ("cnc-02", CncMill(base_temp=55, overheat_cycle=True)),
        4: ("comp-c2", AirCompressor()),
    }
    PlantSimulator(modbus_machines).serve(host, int(os.getenv("MODBUS_PORT", "5020")))


if __name__ == "__main__":
    main()
