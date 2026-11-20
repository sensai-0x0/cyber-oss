"""Cluster -> Intel -> Mitigate StateGraph with bounded cyclic refinement."""

from langgraph.graph import END, START, StateGraph

from orchestrator.cluster import cluster_event
from orchestrator.policy import approve_policy, synthesize_policy
from orchestrator.score import route_after_intel, score_threat_intel
from orchestrator.state import GraphState


def build_graph(checkpointer=None):
    """Compile the graph; supply a durable checkpointer outside tests."""
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
    builder.add_edge("approval", END)
    return builder.compile(checkpointer=checkpointer)
