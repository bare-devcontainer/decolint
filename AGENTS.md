# Repository Guidelines

- Use English for all documentation and comments.
- PR titles must follow Conventional Commits format:
  - Allowed types: `feat`, `fix`, `ci`, `chore`, `test`, `docs`
  - The scope is optional. Examples:
    - `feat(cli): add new feature`
    - `ci: pin action SHAs`
    - `chore: update renovate config`
- Use minimal external dependencies.

## Workflows

- Use `make` to run workflows locally. Run `make help` to see the full list of available targets.
- Run `make lint` after making changes to ensure code quality.

## Testing

- Name a test after the function under test: `TestTarget` or `TestTarget_XXX`
  (e.g. `TestInstallOrder`, `TestInstallOrder_OCIOrder`). Name it for the
  function it calls.
- Collapse same-shaped cases into table-driven subtests; keep tests with different targets separate.
- Use the reserved `.invalid` TLD for placeholder hosts in test data.
- Measure test coverage with `make coverage` after making changes. Ensure coverage does not decrease.

## GitHub Actions

- Pin every action to a full commit SHA with a `# vX.Y.Z` comment,
  e.g. `uses: actions/checkout@9c091bb21b7c1c1d1991bb908d89e4e9dddfe3e0 # v7.0.0`.
- Set workflow-level permissions to empty (`permissions: {}`) and grant
  the minimum required permissions per job.
- Set `persist-credentials: false` on `actions/checkout`.

## Dev Container Specification

- The Dev Container specification lives at
  [containers.dev](https://containers.dev/). Consult it when
  implementing or reviewing lint rules, to confirm behavior matches
  the spec.
- `devcontainer.json` reference:
  https://containers.dev\/implementors/json_reference/
- Features specification:
  https://containers.dev/implementors/features/
- Features distribution (OCI packaging) specification:
  https://containers.dev/implementors/features-distribution/
- Merge logic (image metadata) reference:
  https://containers.dev/implementors/spec/
- Templates specification:
  https://containers.dev/implementors/templates/
- Where the spec prose and the reference implementation
  ([`devcontainers/cli`](https://github.com/devcontainers/cli)) disagree, the
  implementation wins: decolint tracks the real tooling's behavior. Specific
  conflicts are documented as inline comments at the relevant code (see the
  install-order sort in `feature/order.go`).

## Documentation and comments

- For the same information, prefer the shortest, simplest wording. Trim words, clauses, and sentences that add no meaning.
- Documentation comments should explain the contract of a function/package, not the implementation details.
- Inline comments should be used to explain "why" something is done, not "what" is done. The code itself should be clear enough to convey the "what".
- All comments should be important for future maintainers. Do not add comments only meaningful to the current author/pull request.
- Give the reader the orientation they need before reading the code — the overall picture and the caveats that are not obvious from the code — and leave out anything the signature, types, or surrounding code already make clear.
- State each fact in one place. When a comment would restate another function's or package's contract, link to it (e.g. a Go doc link `[Name]`) instead of repeating it.
- Prefer a bullet list to a single long sentence when enumerating several forms, cases, or alternatives.
- Keep adjacent paragraphs distinct; each should add something the others do not.

User-facing documentation (the README and CLI help) additionally:

- Is written for people who use decolint, not those who develop it: document observable behavior and the choices a user has to make, and omit internal mechanisms and implementation choices.
- Does not explain what the reader already knows from their own configuration or usage.
- Does not include notes whose meaning depends on an internal invariant the reader cannot see — a caveat that only makes sense as a contrast to how decolint is built internally.
