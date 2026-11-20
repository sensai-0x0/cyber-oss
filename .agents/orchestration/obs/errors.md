# Agents record error summaries here in the format uuid,description,suggested fix

**Status:** active · **Derived from:** agent error reports · **Updated:** 2026-11-21

,validate_outlines rejected pasted policy values in spec/STATE.md:32 and kept CI red,Cite keys and a commit instead of numbers; fixed on branch fix/triage-errors
,git push --delete returns HTTP 403 for remote branches in the cloud session,Delete merged branches in the GitHub UI or from a local machine
,No Docker daemon in the session so images cannot be built or pulled,Build and run the healthcheck on a machine with Docker before merging .docker/falkordb
,Docker Hub registry API returned TOOMANYREQUESTS for manifest lookups,Authenticate to Docker Hub or retry later; confirm the pinned digest on first build
,docker compose config failed when FALKORDB_PASSWORD was unset because the variable is required,CI and docs now pass FALKORDB_PASSWORD=ci-only for config checks
,Graphiti stock Anthropic client forces tool_choice and sends temperature which current Claude models reject,Write a small LLM client subclass in Phase 3 of plans/graph-store
