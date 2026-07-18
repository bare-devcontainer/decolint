# Repository Guidelines

- Use English for all documentation and comments.
- PR titles must follow Conventional Commits format:
  - Allowed types: `feat`, `fix`, `ci`, `chore`, `test`, `docs`
  - The scope is optional. Examples:
    - `feat(cli): add new feature`
    - `ci: pin action SHAs`
    - `chore: update renovate config`
- Use minimal external dependencies.
- Documentation comments should explain the contract of a function/package, not the implementation details.
- Inline comments should be used to explain "why" something is done, not "what" is done. The code itself should be clear enough to convey the "what".
- All comments should be important for future maintainers. Do not add comments only meaningful to the current author/pull request.

## Workflows

- Use `make` to run workflows locally. Run `make help` to see the full list of available targets.
- Run `make lint` after making changes to ensure code quality.

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
- Templates specification:
  https://containers.dev/implementors/templates/
