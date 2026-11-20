import importlib.util
from pathlib import Path

SCRIPT = Path(__file__).parents[1] / "validate_skills.py"
SPEC = importlib.util.spec_from_file_location("validate_skills", SCRIPT)
validator = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(validator)


def skill(
    name="example", description="A sufficiently detailed skill description for validation."
) -> str:
    return (
        f"---\nname: {name}\ndescription: {description}\n---\n\n"
        "# Workflow\n\nDo the work and hand off a result.\n"
    )


def test_valid_skill():
    errors = validator.validate_text("example/SKILL.md", skill())
    assert errors == []


def test_rejects_missing_description_and_name_mismatch():
    text = "---\nname: other\n---\nBody\n"
    errors = validator.validate_text("example/SKILL.md", text)
    assert any("description" in error for error in errors)
    assert any("must match directory" in error for error in errors)


def test_rejects_invalid_skill_name():
    errors = validator.validate_text("Bad_Name/SKILL.md", skill(name="Bad_Name"))
    assert any("lowercase kebab-case" in error for error in errors)


def test_rejects_more_than_five_skills(tmp_path):
    root = tmp_path / "skills"
    for index in range(6):
        path = root / f"skill-{index}"
        path.mkdir(parents=True)
        (path / "SKILL.md").write_text(skill(name=f"skill-{index}"))
    errors = validator.validate_tree(root)
    assert any("at most 5" in error for error in errors)


def test_rejects_unknown_skill_call(tmp_path):
    root = tmp_path / "skills"
    path = root / "builder"
    path.mkdir(parents=True)
    (path / "SKILL.md").write_text(skill(name="builder") + "\nCalls: missing-skill\n")
    errors = validator.validate_tree(root)
    assert any("unknown skill" in error for error in errors)


def test_rejects_skill_call_cycles(tmp_path):
    root = tmp_path / "skills"
    for name, called in (("builder", "validator"), ("validator", "builder")):
        path = root / name
        path.mkdir(parents=True)
        (path / "SKILL.md").write_text(skill(name=name) + f"\nCalls: {called}\n")
    errors = validator.validate_tree(root)
    assert any("cycle" in error for error in errors)
