# Project Maintainers

This document lists the maintainers of **Zigbridge** and describes the project governance, roles, and responsibilities.

---

## Active Maintainers

| Name | GitHub | Role | Focus Areas |
| :--- | :--- | :--- | :--- |
| **Julien Breux** | [@julienbreux](https://github.com/julienbreux) | Founder & Lead Maintainer | Core Architecture, Radio Adapters, Release Management |

---

## Roles & Responsibilities

Maintainers are responsible for the health, stability, and future of the Zigbridge project. Core responsibilities include:

1. **Code Review & Quality Enforcement**:
   - Review pull requests against the project's engineering principles: Go modernization, zero-allocation memory pooling (`sync.Pool`), sub-15ms direct binding performance, and offline-first CDN-free web UI.
   - Ensure test coverage (`make test`) and static analysis (`make lint`) remain green.
2. **Issue Triage**:
   - Triage reported bugs, coordinator hardware compatibility reports, and feature requests.
   - Tag issues and guide contributors toward relevant documentation and specifications in `specs/`.
3. **Release Management**:
   - Maintain the changelog, cut semantic version tags, and oversee automated multi-architecture releases (`.goreleaser.yaml`).
4. **Security Response**:
   - Triage and coordinate security reports in accordance with [`SECURITY.md`](SECURITY.md).
5. **Community & Culture**:
   - Foster an inclusive, constructive, and respectful environment in compliance with the [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

---

## Decision-Making Process

Zigbridge follows a **consensus-seeking model** grounded in clear specifications:

- **Minor Changes & Bug Fixes**:
  - Can be approved and merged by any maintainer once CI passes and acceptance criteria are satisfied.
- **Architectural & Public API Changes**:
  - Major features, new radio adapter types, changes to REST/MQTT contracts, or alterations to the direct binding engine require an approved specification in the [`specs/`](specs/) directory before implementation.
  - Discussion takes place openly on GitHub issues or pull requests under a **lazy consensus** principle (no sustained objections within 72 hours).

---

## Becoming a Maintainer

We welcome contributors who demonstrate sustained commitment to the project. Criteria for nomination as a maintainer include:

- A track record of high-quality contributions (pull requests, code reviews, issue triaging, or hardware validation).
- Strong understanding of the codebase, idiomatic Go standards, and Zigbee/ZCL protocols.
- Exemplary adherence to our [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).

Nomination is proposed by an existing maintainer and confirmed through consensus.
