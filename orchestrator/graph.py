"""Cluster -> Intel -> Mitigate StateGraph with bounded cyclic refinement."""

import sqlite3

from langgraph.checkpoint.sqlite import SqliteSaver
from langgraph.graph import END, START, StateGraph

from orchestrator.cluster import cluster_event
from orchestrator.ledger import LedgerWriter, make_record_node
from orchestrator.policy import approve_policy, synthesize_policy
from orchestrator.score import route_after_intel, score_threat_intel
from orchestrator.state import GraphState


def build_graph(checkpointer=None, ledger: LedgerWriter | None = None):
    """Compile the graph; supply a durable checkpointer outside tests.

    A ledger, when given, gets one appended record per completed run. It is write-only:
    the graph never reads it, and a failed write leaves the decision unchanged.
    """
    builder = StateGraph(GraphState)
    builder.add_node("cluster", cluster_event)
    builder.add_node("intel", score_threat_intel)
    builder.add_node("mitigate", synthesize_policy)
    builder.add_node("approval", approve_policy)
    builder.add_edge(START, "cluster")
    builder.add_edge("cluster", "intel")
    builder.add_conditional_edges(
        "intel", route_after_intel, {"cluster": "cluster", "mitigate": "mitigate"}
    )
    builder.add_edge("mitigate", "approval")
    if ledger is None:
        builder.add_edge("approval", END)
    else:
        builder.add_node("record", make_record_node(ledger))
        builder.add_edge("approval", "record")
        builder.add_edge("record", END)
    return builder.compile(checkpointer=checkpointer)


def build_sqlite_checkpointer(db_path: str) -> SqliteSaver:
    """Return a durable, file-backed checkpointer option for `build_graph`.

    This is an opt-in alternative to the in-memory `MemorySaver` used in
    tests and by callers that don't need restart-safe interrupts; it is
    NOT wired in as `build_graph`'s default. Choosing SQLite (vs. Postgres
    or another backend) for production is a pending human checkpoint
    (see plans/planv1.0/PHASE2.md P2.2) and must not be inferred as
    approved from the mere existence of this helper.

    A fresh `SqliteSaver` built from the same `db_path` (e.g. after a
    process restart) will resume from any checkpoints already persisted
    to that file, including a pending `interrupt()`.
    """
    connection = sqlite3.connect(db_path, check_same_thread=False)
    return SqliteSaver(connection)
