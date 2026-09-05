import dagre from "@dagrejs/dagre";
import {
  Background,
  Controls,
  Handle,
  Position,
  ReactFlow,
  type Edge,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import "@xyflow/react/dist/style.css";
import { useMemo } from "react";
import { buildGraph, type SchemaField } from "../../lib/schema";

export type SchemaGraphProps = {
  /** The full, unfiltered, uncollapsed row list — the graph always shows
   * everything; collapsing is a browse-mode concern for the list view,
   * not this one (per the explicit request to see the whole structure
   * here). */
  rows: SchemaField[];
  selected: string[];
  onToggle: (path: string) => void;
};

const NODE_WIDTH = 150;
const NODE_HEIGHT = 46;

type FieldNodeData = { label: string; type: string; description?: string };

function FieldNode({ data }: NodeProps<Node<FieldNodeData>>) {
  return (
    <div className="px-2.5 py-1.5 rounded-lg border border-border bg-surface shadow-sm min-w-[130px] max-w-[180px]">
      <Handle type="target" position={Position.Left} className="!bg-muted !border-0 !w-1.5 !h-1.5" />
      <div className="text-[11px] font-mono font-semibold text-ink truncate">{data.label}</div>
      <div className="text-[9.5px] text-muted truncate">{data.type}</div>
      <Handle type="source" position={Position.Right} className="!bg-muted !border-0 !w-1.5 !h-1.5" />
    </div>
  );
}

const nodeTypes = { field: FieldNode };

function layout(nodes: Node[], edges: Edge[]): Node[] {
  const g = new dagre.graphlib.Graph();
  g.setDefaultEdgeLabel(() => ({}));
  g.setGraph({ rankdir: "LR", nodesep: 14, ranksep: 64 });
  for (const n of nodes) g.setNode(n.id, { width: NODE_WIDTH, height: NODE_HEIGHT });
  for (const e of edges) g.setEdge(e.source, e.target);
  dagre.layout(g);
  return nodes.map((n) => {
    const pos = g.node(n.id);
    return { ...n, position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 } };
  });
}

/**
 * Node-and-edge view of a schema, built to be *edited* rather than just
 * read — every edge represents whether its target field passes through
 * (solid green) or has been toggled off (dashed gray), and clicking an
 * edge toggles that field the same way the list view's pill switch does.
 * A generic JSON visualizer (jsoncrack and similar) doesn't support that:
 * those render a read-only structure with no concept of "this connection
 * is cut", so this is a small purpose-built graph instead of an embed.
 */
export function SchemaGraph({ rows, selected, onToggle }: SchemaGraphProps) {
  const { nodes, edges } = useMemo(() => {
    const { nodes: graphNodes, edges: graphEdges } = buildGraph(rows);

    const flowNodes: Node[] = graphNodes.map((n) => ({
      id: n.id,
      type: "field",
      data: { label: n.label, type: n.type, description: n.description },
      position: { x: 0, y: 0 },
      // Declaring the size up front (we already know it, and use the same
      // constants for the dagre layout below) marks the node as measured
      // immediately -- otherwise fitView can run before an async
      // ResizeObserver round-trip measures every node, freezing on
      // whatever subset happened to be measured first and stranding the
      // rest off-screen forever on a graph this size (found on the real
      // 15-level-deep fixture, not reproducible with only 2-3 nodes).
      width: NODE_WIDTH,
      height: NODE_HEIGHT,
    }));

    const flowEdges: Edge[] = graphEdges.map((e) => {
      const dropped = selected.includes(e.target);
      return {
        id: e.id,
        source: e.source,
        target: e.target,
        type: "smoothstep",
        style: {
          stroke: dropped ? "var(--color-track-off)" : "var(--color-success)",
          strokeWidth: 2,
          strokeDasharray: dropped ? "5 4" : undefined,
        },
      };
    });

    return { nodes: layout(flowNodes, flowEdges), edges: flowEdges };
  }, [rows, selected]);

  return (
    <div className="flex-1 min-h-0 border border-border rounded-lg overflow-hidden">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
        fitView
        // React Flow's default minZoom (0.5) can't zoom out far enough to
        // fit a schema graph many nodes wide, so fitView clamps and
        // centers on the middle of the graph instead of showing it all
        // (found on the real 15-level-deep fixture, invisible with only a
        // couple of nodes).
        minZoom={0.05}
        onEdgeClick={(_, edge) => onToggle(edge.target)}
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  );
}
