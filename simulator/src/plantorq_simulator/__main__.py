"""Composition root: choose which machines exist and start the server."""

import logging
import os

from .profiles import AirCompressor, CncMill, WeldingRobot
from .server import PlantSimulator


def main() -> None:
    logging.basicConfig(
        level=os.getenv("LOG_LEVEL", "INFO").upper(), format="%(asctime)s %(levelname)s %(name)s %(message)s"
    )
    # Unit 5 (press-p04) is deliberately absent: the agent polls it and the
    # dashboard shows how an unreachable machine is handled.
    machines = {
        1: ("cnc-01", CncMill()),
        2: ("cnc-02", CncMill(base_temp=55, overheat_cycle=True)),
        3: ("robot-07", WeldingRobot()),
        4: ("comp-c2", AirCompressor()),
    }
    PlantSimulator(machines).serve(os.getenv("BIND_HOST", "0.0.0.0"), int(os.getenv("MODBUS_PORT", "5020")))


if __name__ == "__main__":
    main()
