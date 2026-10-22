import random

from plantorq_simulator.profiles import AirCompressor, CncMill, Snapshot
from plantorq_simulator.registers import REGISTER_MAP, encode


def run(profile, seconds):
    return [profile.step(1.0) for _ in range(seconds)]


def test_overheating_cnc_crosses_threshold_and_recovers():
    temps = [s.temperature for s in run(CncMill(base_temp=55, overheat_cycle=True, rng=random.Random(1)), 240)]
    assert max(temps) > 70, "demo machine must trigger the overheat rule"
    assert temps[-1] < 70, "and cool down again within one cycle"


def test_normal_cnc_stays_below_threshold():
    assert all(s.temperature < 70 for s in run(CncMill(rng=random.Random(1)), 600))


def test_compressor_has_periodic_vibration_spikes():
    vib = [s.vibration for s in run(AirCompressor(rng=random.Random(1)), 180)]
    assert max(vib) > 5.5 and min(vib) < 3.5


def test_encode_scales_and_clamps():
    snap = Snapshot(temperature=73.46, power_kw=22.5, vibration=-1, current=9999, spindle_rpm=9000, running=True)
    words = encode(snap)
    assert len(words) == len(REGISTER_MAP)
    assert words[0] == 735  # 73.46 / 0.1, rounded
    assert words[2] == 0  # negative clamped
    assert words[3] == 0xFFFF  # overflow clamped
    assert words[5] == 1
