# React Flow for the Automation editor

The Automation editor is built on React Flow (`@xyflow/react`, MIT). The `automations.json` document maps onto its model with no translation: a Step is a node with its `id` and inline `position`, and an edge joins a source handle to a target handle, the Step's named handles. Each Step kind is a React component that renders its params form inside the node. The Code Step's declared outputs become handles at runtime (`useUpdateNodeInternals`), and `isValidConnection` refuses bad joins before they exist. The [Automation editor UX](https://github.com/llehouerou/oiko/issues/25) prototype (branch `prototype/automation-editor`) confirmed this on flows 5a and 6c. Its dependency `zustand` resolves to 4.5.7, so the React 19 conflict reported in xyflow#5229 does not apply.

## Considered Options

- **Rete.js v2**: MIT and smaller, but it models sockets and connections, so the document would need a translation layer. It is assembled from 4+ packages and has no documented way to add sockets at runtime, which the Code Step needs.
- **An outline editor with a read-only generated map** (prototype variant C): no graph-editing library in the write path and no hand positions. Rejected in favour of editing on the graph itself. A generated layout of 5a is also a long thin strip.
- **BaklavaJS, Drawflow, JointJS, litegraph.js**: Vue-only, no React binding, a paid tier, or canvas-only.
