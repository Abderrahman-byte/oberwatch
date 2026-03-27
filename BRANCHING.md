# Branching Strategy

Oberwatch uses GitHub Flow with a protected `staging` branch in front of `main`.

## Branch Overview

- `main` is the production-ready branch. Every merge to `main` should be stable and eligible for release.
- `staging` is the integration branch. Feature work lands here first for combined testing.
- `feature/*` branches are short-lived branches created from `staging` for normal work.
- `hotfix/*` branches are short-lived branches created from `main` for urgent production fixes.

## Feature Workflow

Create each feature branch from `staging`, not from `main`.

```bash
git checkout staging
git pull origin staging
git checkout -b feature/short-description
```

Develop on the feature branch, push it, and open a pull request targeting `staging`.

```bash
git push origin feature/short-description
```

Every pull request to `staging` must pass CI before merge.

## Promoting Staging to Main

When `staging` is stable, open a pull request from `staging` to `main`.

- Merge to `main` only after CI passes.
- Use squash merge to keep history clean.
- Treat every merge to `main` as production-ready.

Merges to `main` publish the `latest` container image. Merges to `staging` publish the `staging` container image.

## Tagged Releases

Tagged releases are created from `main` only.

- Create a tag like `v0.1.0` on `main`.
- Pushing the tag runs the release workflow.
- The release workflow publishes binaries plus container images to GHCR and Docker Hub.
- Container tags include the full version, the minor version alias, and `latest`.

## Hotfix Workflow

Hotfixes start from `main` because they address production issues.

```bash
git checkout main
git pull origin main
git checkout -b hotfix/short-description
```

Open the hotfix pull request against `main`. After it merges, cherry-pick the same fix onto `staging` so the branches stay aligned.

## Summary Rules

- Create `feature/*` branches from `staging`.
- Open normal pull requests into `staging`.
- Promote `staging` into `main` when the integration branch is stable.
- Create release tags from `main` only.
- Create `hotfix/*` branches from `main`, merge them to `main`, then backport to `staging`.
