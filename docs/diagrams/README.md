# Sequence Diagrams

This directory contains sequence diagram markup files that document the interactions and workflows within the MultiCluster Observability Operator (MCO).

## Organization

The diagrams are organized into the following categories:

### Architecture (`architecture/`)
High-level architectural diagrams showing system setup and component relationships:
- **observability-bootstrapping-hub.md** - Hub cluster observability setup and initialization
- **observability-bootstrapping-managed.md** - Managed cluster bootstrap process

### Components (`components/`)
Detailed component interaction diagrams showing how specific parts of the system work together:
- **metrics-collection-alert-forwarding.md** - Metrics collection and alert forwarding flow

### Workflows (`workflows/`)
Process and workflow diagrams showing end-to-end system behaviors:
- **metrics-collection-status-propagation.md** - Overall status propagation workflow across the system
- **observability-placement-delete-flow.md** - Managed cluster detach/disable observability process

## How to Use These Diagrams

### Viewing and Editing
All `.md` files in this directory contain Mermaid sequence diagram markup that renders directly in GitHub.

To edit or visualize outside of GitHub:
1. Open [Mermaid Live Editor](https://mermaid.live/)
2. Copy the content inside the `mermaid` code block from any `.md` file in this directory
3. Paste it into the editor
4. The diagram will be rendered automatically
5. You can edit the markup and see real-time updates to the diagram

### Creating New Diagrams
When creating new sequence diagrams:

1. Use the existing files as templates
2. Follow the naming convention: `descriptive-name.md`
3. Wrap the mermaid syntax in a `mermaid` code block:
   ```mermaid
   ...
   ```
4. Place the file in the appropriate subdirectory based on its purpose
5. Update this README.md to document the new diagram

### Markup Syntax
The diagrams use standard [Mermaid Sequence Diagram](https://mermaid.js.org/syntax/sequenceDiagram.html) syntax. Key elements include:

- `title` - Diagram title
- `participant` - System components/actors
- `->` - Synchronous messages
- `-->` - Asynchronous messages
- `==` - Section dividers
- `#color` - Color coding for participants (note: specific color syntax might vary between renderers)

## Contributing

When adding new sequence diagrams:

1. Ensure the diagram accurately represents the current system behavior
2. Use clear, descriptive participant names
3. Include relevant comments and section dividers for complex flows
4. Test the markup on GitHub or Mermaid Live Editor before committing
5. Update this README.md to include the new diagram

## Related Documentation

- [MultiCluster Observability CRD](../MultiClusterObservability-CRD.md) - API reference
- [Debug Guide](../debug.md) - Troubleshooting information
- [Scale and Performance](../scale-perf.md) - Performance considerations