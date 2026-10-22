"""Modbus TCP server exposing one unit id per simulated machine."""

from __future__ import annotations

import logging
import threading

from pymodbus.datastore import ModbusSequentialDataBlock, ModbusServerContext, ModbusSlaveContext
from pymodbus.server.sync import StartTcpServer

from .profiles import MachineProfile
from .registers import REGISTER_MAP, encode

log = logging.getLogger(__name__)

HOLDING_REGISTERS = 3  # Modbus function code / pymodbus datastore key


class PlantSimulator:
    """Owns the machines and keeps their registers up to date."""

    def __init__(self, machines: dict[int, tuple[str, MachineProfile]], tick: float = 1.0) -> None:
        self._machines = machines
        self._tick = tick
        self._stop = threading.Event()
        slaves = {
            unit: ModbusSlaveContext(hr=ModbusSequentialDataBlock(0, [0] * len(REGISTER_MAP)), zero_mode=True)
            for unit in machines
        }
        # single=False: each unit id is a separate device on the same socket.
        self.context = ModbusServerContext(slaves=slaves, single=False)

    def update_once(self) -> None:
        for unit, (name, profile) in self._machines.items():
            snapshot = profile.step(self._tick)
            self.context[unit].setValues(HOLDING_REGISTERS, 0, encode(snapshot))
            log.debug("unit %d (%s): %s", unit, name, snapshot)

    def _update_loop(self) -> None:
        while not self._stop.wait(self._tick):
            self.update_once()

    def serve(self, host: str, port: int) -> None:
        self.update_once()
        threading.Thread(target=self._update_loop, daemon=True).start()
        log.info("simulating %d machines on %s:%d", len(self._machines), host, port)
        try:
            StartTcpServer(context=self.context, address=(host, port))
        finally:
            self._stop.set()
