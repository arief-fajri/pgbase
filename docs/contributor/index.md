# Development

<DocMeta audience="Contributor" status="stable" verified="v0.5.2" />

Welcome to the PG-BASE development section. This area is for contributors, maintainers, and developers who want to understand or extend the platform itself.

## Quick links

| Resource | Description |
|----------|-------------|
| [Development Setup](./developing.md) | Local environment, prerequisites, build commands |
| [Contributing Guide](./contributing.md) | How to prepare a PR |
| [Release Process](./releasing.md) | Tag-driven release workflow |

## Architecture

Understand how the system is built:

- [System Overview](../architecture/overview.md) — components, layers, flowchart
- [Backend Layers](../architecture/backend-layers.md) — bootstrap chain, dual-pool, middleware order
- [Audit Trail](../architecture/audit-design.md) — write/read trail design

## Engineering Methodology

PG-BASE follows a system-thinking engineering loop. These documents define how we build, verify, and maintain the system:

| Phase | Document | Purpose |
|-------|----------|---------|
| 1 | [Platform Design](./methodology/platform-design.md) | What we're building, system boundaries, invariants |
| 2 | [Quality Guardrails](./methodology/guardrails.md) | What is never allowed |
| 3 | [Observability](./methodology/observability.md) | How we prove guard rails hold |
| 4 | [Evaluation Checklists](./methodology/checklists.md) | Release readiness gates |

If a test fails, use [Failure Analysis](./methodology/failure-modes.md) to classify the failure before fixing.

## Project resources

- [Upstream Tracking](./upstream.md) — PG-BASE vs PocketBase status
- [Roadmap](./roadmap.md) — product and development roadmap
