# Agents record error summaries here in the format uuid,description,suggested fix

**Status:** active · **Derived from:** agent error reports · **Updated:** 2026-11-21

739a96c6-f998-4e1d-8435-b0d4dfd273ef,validate_outlines rejected pasted policy values in spec/STATE.md:32 and kept CI red,Cite keys and a commit instead of numbers; fixed on branch fix/triage-errors
0c9f83e7-725e-474e-aad4-160c05e81dc3,git push --delete returns HTTP 403 for remote branches in the cloud session,Delete merged branches in the GitHub UI or from a local machine
7fa31f19-698c-4f1f-923a-0e217d2cfd70,No Docker daemon in the session so images cannot be built or pulled,Build and run the healthcheck on a machine with Docker before merging .docker/falkordb
fa027e63-1ac2-437e-b252-c940b2bca7b1,Docker Hub registry API returned TOOMANYREQUESTS for manifest lookups,Authenticate to Docker Hub or retry later; confirm the pinned digest on first build
55a3ad1c-634a-4939-8615-55e938b3166a,docker compose config failed when FALKORDB_PASSWORD was unset because the variable is required,CI and docs now pass FALKORDB_PASSWORD=ci-only for config checks
0adebf0d-2964-43a6-a286-675c4e10ad3a,Graphiti stock Anthropic client forces tool_choice and sends temperature which current Claude models reject,Write a small LLM client subclass in Phase 3 of plans/graph-store
a883eb6c-13ac-474c-a799-2f034f57e6a2,Three test_classify_target_table cases fail on CPython 3.13 because ipaddress renders IPv4-mapped IPv6 differently; pyproject allows >=3.11<3.14 so a bare uv run picks 3.13 and the suite is red,Either pin requires-python to <3.13 or derive the expected strings in the test rather than hardcoding them; CI and CLAUDE.md both pass --python 3.12 so the gate never saw it
