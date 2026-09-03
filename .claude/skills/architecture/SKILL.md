---
name: architecture
description: >
  Software architecture guidance and documentation. Use when designing or
  changing system architecture, evaluating technologies or frameworks,
  making significant technical decisions, defining components and boundaries,
  or creating/updating architecture documentation and Architecture Decision
  Records (ADRs) under /docs/architecture.
---

# Software Architecture

You are responsible for helping design, evaluate, and document the architecture
of this software project.

Priorities, in order:

1. Correctness
2. Simplicity
3. Maintainability
4. Reliability
5. Security
6. Operability and observability
7. Performance and scalability
8. Development productivity
9. Cost

Do not optimize for theoretical scalability or complexity unless the project
requirements justify it.

## Architecture Documentation

All persistent architecture documentation belongs under:

    /docs/architecture/

Architecture documentation must be written in Markdown.

Use architecture documents to describe the current or intended structure of
the system.

Typical documents include:

- `overview.md` — high-level architecture and system boundaries
- `components.md` — major components and their responsibilities
- `data.md` — data architecture and data flows
- `deployment.md` — deployment/runtime architecture
- `security.md` — security architecture
- `observability.md` — logging, metrics, tracing, and monitoring
- `integration.md` — external systems and integration patterns
- `scalability.md` — scaling and capacity considerations
- `testing.md` — architectural testing strategy

Do not create documents merely to follow the list above. Create them when the
project actually needs the information documented.

Prefer a small number of useful documents over a large documentation tree.

## Architecture Decisions

Important architectural decisions belong under:

    /docs/architecture/decisions/

Use Architecture Decision Records (ADRs) for decisions that:

- affect system structure
- introduce or remove a major technology
- establish an architectural pattern
- create an important dependency
- affect scalability, reliability, security, or operations
- are expensive or difficult to reverse
- establish a convention that future developers are expected to follow
- involve meaningful trade-offs between alternatives

Do NOT create an ADR for trivial implementation details.

Examples of decisions that deserve an ADR:

- choosing PostgreSQL vs SQLite
- choosing Phoenix vs ASP.NET Core
- choosing REST vs gRPC
- choosing a message broker
- choosing Kubernetes vs Nomad
- choosing an authentication mechanism
- choosing a persistence strategy
- choosing a caching architecture
- choosing synchronous vs asynchronous processing
- choosing a deployment model

## ADR Naming

Use sequential numeric identifiers:

    0001-use-postgresql.md
    0002-use-phoenix.md
    0003-use-redis-for-caching.md

Before creating an ADR:

1. Inspect `/docs/architecture/decisions/`.
2. Determine the next available number.
3. Check whether an existing ADR already covers the decision.
4. If the decision already exists, update or supersede the existing ADR rather
   than creating a duplicate.

Never reuse an ADR number.

## ADR Status

Use one of:

- `Proposed`
- `Accepted`
- `Rejected`
- `Deprecated`
- `Superseded`

When a decision replaces an existing ADR:

1. Create a new ADR.
2. Set the old ADR status to `Superseded`.
3. Link the old ADR to the new ADR.
4. Link the new ADR back to the old ADR.

Do not silently rewrite history.

## ADR Format

Use this format:

```markdown
# ADR-NNNN: Decision Title

- Status: Proposed
- Date: YYYY-MM-DD

## Context

Describe the problem that requires a decision.

Include:

- business requirements
- technical requirements
- constraints
- known assumptions
- relevant non-functional requirements
- expected scale
- operational constraints

Do not describe the solution yet.

## Decision

State the decision clearly and unambiguously.

Explain what will be used and how it will be used.

## Alternatives Considered

### Alternative A

Brief description.

Advantages:
- ...

Disadvantages:
- ...

### Alternative B

Brief description.

Advantages:
- ...

Disadvantages:
- ...

### Alternative C

Brief description.

Advantages:
- ...

Disadvantages:
- ...

## Decision Criteria

Evaluate alternatives against criteria relevant to this project.

Typical criteria:

- Functional fit
- Performance
- Scalability
- Reliability
- Security
- Operational complexity
- Maintainability
- Developer experience
- Ecosystem maturity
- Integration requirements
- Resource consumption
- Cost
- Reversibility

Do not include irrelevant criteria merely to make the comparison look
comprehensive.

## Rationale

Explain why the selected option is preferable given the actual project
constraints.---
name: architecture
description: 
  This skill provides guidance on software architecture, including design patterns, best practices, and architectural principles. It can help you make informed decisions about system structure, scalability, and maintainability.
---



Focus on trade-offs rather than generic claims.

## Consequences

### Positive

- ...

### Negative

- ...

### Risks

- ...

### Follow-up

- ...

## Validation

If the decision requires empirical validation, document how it should be
validated.

Examples:

- proof of concept
- benchmark
- load test
- failure test
- security review
- migration test

Record actual findings here when validation has been performed.

## References

Link to relevant documentation, research, existing architecture documents,
or other ADRs.

## Clean Architecture

Prefer Clean Architecture principles when designing the system.

Clean Architecture should be treated as a set of principles, not as a requirement
to reproduce a specific folder structure or framework template.

The primary goals are:

- separation of concerns
- dependency inversion
- independent business rules
- explicit architectural boundaries
- testability
- maintainability
- replaceable infrastructure
- reduced coupling to frameworks and external systems

### Dependency Rule

Dependencies should point inward toward the core business logic.

Prefer a structure conceptually similar to:

    Domain
       ↑
    Application
       ↑
    Infrastructure / Adapters
       ↑
    Presentation / Entry Points

The exact naming and number of layers may differ depending on the project.

Core business logic should not depend directly on:

- web frameworks
- databases
- message brokers
- external APIs
- cloud providers
- infrastructure libraries
- UI frameworks
- delivery mechanisms

Infrastructure and frameworks should depend on application/domain abstractions,
rather than the core domain depending on infrastructure implementations.

### Domain Layer

Keep domain logic independent of infrastructure whenever practical.

The domain should contain:

- business rules
- domain entities
- value objects
- domain-specific policies
- domain invariants

Avoid putting framework-specific annotations, persistence concerns, HTTP
concepts, or infrastructure behavior into the domain unless there is a strong
practical reason.

### Application Layer

The application layer should coordinate use cases.

It may contain:

- use cases
- application services
- commands and queries
- ports/interfaces
- transaction boundaries
- authorization decisions
- orchestration of domain operations

Application code should depend on abstractions for external capabilities.

For example:

    Application
        |
        +-- UserRepository
        +-- EmailSender
        +-- PaymentGateway

Implementations belong outside the application core:

    Infrastructure
        |
        +-- PostgresUserRepository
        +-- SmtpEmailSender
        +-- StripePaymentGateway

### Dependency Inversion

When application code needs an external capability, define the abstraction
where it is consumed rather than coupling the application to the concrete
implementation.

Prefer:

    Application -> Repository interface <- Infrastructure implementation

over:

    Application -> PostgreSQL implementation

This makes infrastructure replaceable and makes application behavior easier
to test.

### Boundaries

Make architectural boundaries explicit.

Examples:

- Domain ↔ Application
- Application ↔ Infrastructure
- Application ↔ Presentation
- Internal system ↔ External API
- Service ↔ Database
- Service ↔ Message Broker

Do not allow dependencies to cross boundaries accidentally.

When a boundary is important, enforce it through:

- module/package structure
- dependency rules
- interfaces
- automated architecture tests
- code review conventions

### Avoid Clean Architecture Overengineering

Do not blindly introduce layers, interfaces, DTOs, factories, repositories,
or abstractions simply because Clean Architecture diagrams contain them.

For small applications, a simpler structure may be more appropriate.

For example, do not create:

    IUserRepository
    UserRepository
    UserRepositoryFactory
    UserRepositoryAdapter
    UserRepositoryMapper

unless the abstraction provides a real architectural benefit.

Prefer the simplest design that preserves the required boundaries.

### Framework Independence

Prefer keeping business and application logic independent from the framework.

Framework-specific code should primarily exist at the edges of the system.

For example:

    HTTP Controller
        ↓
    Application Use Case
        ↓
    Domain

rather than:

    HTTP Controller
        ↓
    Framework-specific business logic
        ↓
    Database

Frameworks should be replaceable where the cost of doing so is reasonable.

This does NOT mean avoiding framework features completely. Framework features
are appropriate when they provide significant value and do not create
unacceptable coupling.

### Database Independence

Do not introduce database abstraction solely for theoretical portability.

However, business logic should not depend directly on database-specific
details when those details are not part of the business domain.

Keep persistence concerns at the infrastructure boundary where practical.

Use database-specific features deliberately when they provide meaningful
benefits. Document significant database coupling when it is an architectural
decision.

### External Systems

Treat external systems as boundaries.

Examples:

- payment providers
- cloud services
- third-party APIs
- email providers
- message brokers
- object storage

Define application-facing interfaces when doing so provides meaningful
decoupling.

Keep external API models and infrastructure-specific types from leaking deeply
into the domain.

Map external representations into application/domain representations when
there is a meaningful architectural boundary.

### Testing

Clean Architecture should improve testability.

Prefer being able to test:

1. Domain rules without infrastructure
2. Application use cases without real external services
3. Infrastructure integrations separately
4. End-to-end behavior through the actual system boundaries

Do not mock everything.

Use real implementations when they make tests simpler and more valuable,
especially for databases and other infrastructure where integration behavior
is important.

### Pragmatic Rule

When Clean Architecture principles conflict with simplicity, explicitly
evaluate the trade-off.

Prefer:

    simple + well-bounded

over:

    highly abstract + unnecessarily complex

The objective is not to maximize the number of architectural layers.

The objective is to keep business rules understandable, dependencies
controlled, and infrastructure replaceable where that provides real value.