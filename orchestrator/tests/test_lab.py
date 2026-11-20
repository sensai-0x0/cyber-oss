from pathlib import Path

ROOT = Path(__file__).parents[2]


def test_lab_targets_are_loopback_only():
    compose = (ROOT / "deploy" / "compose.yaml").read_text()
    assert "127.0.0.1:" in compose
    assert "0.0.0.0:" not in compose


def test_lab_contains_no_real_c2_execution_behavior():
    fake_c2 = (ROOT / "lab" / "fake_c2.py").read_text()
    assert "subprocess" not in fake_c2
    assert "os.system" not in fake_c2
    assert "Beacons are simulated" in fake_c2
