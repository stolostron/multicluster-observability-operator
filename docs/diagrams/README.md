# Sequence Diagrams

This directory contains sequence diagram markup files that document the interactions and workflows within the MultiCluster Observability Operator (MCO).

## Organization

The diagrams are organized into the following categories:

### Architecture (`architecture/`)
High-level architectural diagrams showing system setup and component relationships:
- **observability-bootstrapping-hub.txt** - Hub cluster observability setup and initialization
- **observability-bootstrapping-managed.txt** - Managed cluster bootstrap process

### Components (`components/`)
Detailed component interaction diagrams showing how specific parts of the system work together:
- **metrics-collection-alert-forwarding.txt** - Metrics collection and alert forwarding flow
- **metrics-collection-status-propagation.txt** - Status propagation during metrics collection

### Workflows (`workflows/`)
Process and workflow diagrams showing end-to-end system behaviors:
- **status-propagation.txt** - Overall status propagation workflow across the system
- **observability-placement-delete-flow.txt** - Managed cluster detach/disable observability process

## How to Use These Diagrams

### Viewing and Editing
All `.txt` files in this directory contain sequence diagram markup that can be opened and edited on [sequencediagram.org](https://sequencediagram.org).

1. Open [sequencediagram.org](https://sequencediagram.org) in your browser
2. Copy the content from any `.txt` file in this directory
3. Paste it into the editor on sequencediagram.org
4. The diagram will be rendered automatically
5. You can edit the markup and see real-time updates to the diagram

### Creating New Diagrams
When creating new sequence diagrams:

1. Use the existing files as templates for markup syntax
2. Follow the naming convention: `descriptive-name.txt`
3. Place the file in the appropriate subdirectory based on its purpose
4. Update this README.md to document the new diagram

### Markup Syntax
The diagrams use the sequence diagram markup syntax supported by sequencediagram.org. Key elements include:

- `title` - Diagram title
- `participant` - System components/actors
- `->` - Synchronous messages
- `-->` - Asynchronous messages
- `==` - Section dividers
- `#color` - Color coding for participants

## Contributing

When adding new sequence diagrams:

1. Ensure the diagram accurately represents the current system behavior
2. Use clear, descriptive participant names
3. Include relevant comments and section dividers for complex flows
4. Test the markup on sequencediagram.org before committing
5. Update this README.md to include the new diagram

## Related Documentation

- [MultiCluster Observability CRD](../MultiClusterObservability-CRD.md) - API reference
- [Debug Guide](../debug.md) - Troubleshooting information
- [Scale and Performance](../scale-perf.md) - Performance considerations 