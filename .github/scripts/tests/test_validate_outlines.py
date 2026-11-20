import importlib.util
from pathlib import Path

SCRIPT = Path(__file__).parents[1] / "validate_outlines.py"
SPEC = importlib.util.spec_from_file_location("validate_outlines", SCRIPT)
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)

HEADER = "**Status:** authoritative · **Derived from:** `spec/` · **Updated:** 2026-11-21\n"


def build_root(tmp_path: Path, body: str, *, policy: str | None = None) -> Path:
    (tmp_path / ".agents").mkdir(parents=True)
    (tmp_path / ".agents" / "outline.md").write_text(HEADER + body)
    (tmp_path / "spec").mkdir(parents=True)
    (tmp_path / "spec" / "ARCHITECTURE.md").write_text(HEADER)
    if policy is not None:
        (tmp_path / "orchestrator").mkdir(parents=True)
        (tmp_path / "orchestrator" / "policy.yaml").write_text(policy)
    return tmp_path


def test_clean_outline_passes(tmp_path):
    root = build_root(tmp_path, "\nSee [architecture](../spec/ARCHITECTURE.md).\n")
    errors, warnings = validator.validate_tree(root)
    assert errors == []
    assert warnings == []


def test_rejects_dead_relative_link(tmp_path):
    root = build_root(tmp_path, "\nSee [gone](../spec/MISSING.md).\n")
    errors, _ = validator.validate_tree(root)
    assert any("dead link" in error for error in errors)


def test_rejects_backticked_path_that_does_not_exist(tmp_path):
    root = build_root(tmp_path, "\nRead `pkg/cidr/feistel.go` before editing.\n")
    errors, _ = validator.validate_tree(root)
    assert any("does not exist" in error for error in errors)


def test_planned_marker_exempts_a_missing_path(tmp_path):
    root = build_root(tmp_path, "\n`pkg/cidr/feistel.go` (planned) per ADR 0003.\n")
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_ignores_paths_inside_code_fences(tmp_path):
    root = build_root(tmp_path, "\n```\n`pkg/cidr/feistel.go`\n```\n")
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_ignores_bare_filenames_without_a_directory(tmp_path):
    root = build_root(tmp_path, "\nThresholds live in `policy.yaml` only.\n")
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_rejects_pasted_distinctive_threshold(tmp_path):
    root = build_root(
        tmp_path,
        "\nMitigate above 0.8 confidence.\n",
        policy="version: v1\nmitigation_confidence: 0.8\n",
    )
    errors, _ = validator.validate_tree(root)
    assert any("pasted" in error for error in errors)


def test_allows_threshold_cited_by_key_name(tmp_path):
    root = build_root(
        tmp_path,
        "\nMitigate above `policy.yaml: mitigation_confidence`.\n",
        policy="version: v1\nmitigation_confidence: 0.8\n",
    )
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_small_integers_are_not_treated_as_thresholds(tmp_path):
    root = build_root(
        tmp_path,
        "\nA 4-round Feistel network, at most 2 refinements.\n",
        policy="version: v1\nmax_refinement_cycles: 2\nbroad_prefix_threshold: 24\n",
    )
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_missing_provenance_is_a_warning_not_an_error(tmp_path):
    (tmp_path / ".agents").mkdir(parents=True)
    (tmp_path / ".agents" / "outline.md").write_text("# No header\n\nBody.\n")
    errors, warnings = validator.validate_tree(tmp_path)
    assert errors == []
    assert any("missing provenance" in warning for warning in warnings)


def test_rejects_unrecognised_status(tmp_path):
    (tmp_path / ".agents").mkdir(parents=True)
    (tmp_path / ".agents" / "outline.md").write_text("**Status:** canonical\n")
    _, warnings = validator.validate_tree(tmp_path)
    assert any("unrecognised status" in warning for warning in warnings)


def test_proposed_status_exempts_a_document_from_path_existence(tmp_path):
    """A design that was never built must be free to name the paths it would have had."""
    (tmp_path / ".agents").mkdir(parents=True)
    (tmp_path / ".agents" / "old-design.md").write_text(
        "**Status:** proposed · **Derived from:** ADR 0003 · **Updated:** 2026-11-21\n\n"
        "The permuter lives in `pkg/cidr/feistel.go`.\n"
    )
    errors, _ = validator.validate_tree(tmp_path)
    assert errors == []


def test_authoritative_status_does_not_exempt_path_existence(tmp_path):
    root = build_root(tmp_path, "\nThe permuter lives in `pkg/cidr/feistel.go`.\n")
    errors, _ = validator.validate_tree(root)
    assert any("does not exist" in error for error in errors)


def test_accepts_a_path_written_relative_to_the_document(tmp_path):
    """`../spec/ARCHITECTURE.md` is as valid a spelling as the root-relative form."""
    root = build_root(tmp_path, "\nSee `../spec/ARCHITECTURE.md` for the layout.\n")
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_ignores_placeholder_templates(tmp_path):
    root = build_root(tmp_path, "\nResults go to `results/<phase>-<date>.json`.\n")
    errors, _ = validator.validate_tree(root)
    assert errors == []


def test_adr_directory_is_exempt_from_path_existence(tmp_path):
    (tmp_path / "spec" / "DECISIONS").mkdir(parents=True)
    (tmp_path / "spec" / "DECISIONS" / "0003-x.md").write_text(
        "- Status: accepted\n\nSuperseded `pkg/kv/store.go` in favour of the store.\n"
    )
    errors, _ = validator.validate_tree(tmp_path)
    assert errors == []
