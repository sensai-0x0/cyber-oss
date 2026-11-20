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


def _compose():
    import yaml

    return yaml.safe_load((ROOT / "deploy" / "compose.yaml").read_text())


def test_falkordb_has_no_published_ports_and_sits_on_internal_network_only():
    compose = _compose()
    service = compose["services"]["falkordb"]
    assert "ports" not in service
    assert service["networks"] == ["graphnet"]
    assert compose["networks"]["graphnet"]["internal"] is True


def test_falkordb_is_opt_in_limited_and_requires_a_password():
    service = _compose()["services"]["falkordb"]
    assert service["profiles"] == ["graph"]
    assert "cpus" in service and "mem_limit" in service
    assert ":?" in service["environment"]["REDIS_ARGS"]


def test_only_falkordb_joins_graphnet():
    services = _compose()["services"]
    joined = [n for n, s in services.items() if "graphnet" in (s.get("networks") or [])]
    assert joined == ["falkordb"]


def test_falkordb_image_is_pinned_by_digest_and_holds_no_secret():
    text = (ROOT / ".docker" / "falkordb" / "Dockerfile").read_text()
    from_line = next(line for line in text.splitlines() if line.startswith("FROM "))
    assert "@sha256:" in from_line and ":latest" not in from_line
    assert "requirepass" not in text.replace("--requirepass ${", "")
