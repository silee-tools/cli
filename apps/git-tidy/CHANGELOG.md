# Changelog

## [0.7.1](https://github.com/silee-tools/cli/compare/git-tidy/v0.7.0...git-tidy/v0.7.1) (2026-06-09)


### Bug Fixes

* **git-tidy:** recover broken worktree with missing .git link ([#82](https://github.com/silee-tools/cli/issues/82)) ([9153164](https://github.com/silee-tools/cli/commit/91531642236ae1c8325227ee4dd8c08a708f3382))

## [0.7.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.6.0...git-tidy/v0.7.0) (2026-06-08)


### Features

* **git-tidy:** 손상 저장소의 깨진 브랜치를 건너뛰고 계속 진행 ([#80](https://github.com/silee-tools/cli/issues/80)) ([be8ea8b](https://github.com/silee-tools/cli/commit/be8ea8b2a9ced06a256f2bf2610cb57c5ddad810))

## [0.6.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.5.0...git-tidy/v0.6.0) (2026-06-05)


### Features

* **git-tidy:** detect absorbed branch cleanup candidates ([#78](https://github.com/silee-tools/cli/issues/78)) ([314e2f5](https://github.com/silee-tools/cli/commit/314e2f5b34c29b40e43bec3255c07c9e5c26cf4a))

## [0.5.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.4.0...git-tidy/v0.5.0) (2026-06-01)


### Features

* **git-tidy:** 기본 선택 한정·사유별 정렬·그룹 TUI 개선 ([#76](https://github.com/silee-tools/cli/issues/76)) ([65d0d3c](https://github.com/silee-tools/cli/commit/65d0d3ce24a46316b184afb99df6ec3662816d7b))

## [0.4.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.3.0...git-tidy/v0.4.0) (2026-05-22)


### Features

* **git-tidy:** add gtidy/gtidy! multi-call shortcuts ([#73](https://github.com/silee-tools/cli/issues/73)) ([7873484](https://github.com/silee-tools/cli/commit/7873484184c29bfde93ad43f67b82e0ce351184b))

## [0.3.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.2.1...git-tidy/v0.3.0) (2026-05-22)


### ⚠ BREAKING CHANGES

* **git-tidy:** scaffold Go module, drop zsh plugin

### Features

* **git-tidy:** add git CLI wrapper ([5dd7fd3](https://github.com/silee-tools/cli/commit/5dd7fd3b90026c7e2d85ff989587e96f00cb71d3))
* **git-tidy:** add hybrid branch classification ([7afb090](https://github.com/silee-tools/cli/commit/7afb0905507ea73b0932ba6121684d74590b520c))
* **git-tidy:** add mode detection and line-based selection ([a670ac7](https://github.com/silee-tools/cli/commit/a670ac720e5fa98a0d485a4deb69903b7d2cccb6))
* **git-tidy:** add pure multi-select model ([bf15d34](https://github.com/silee-tools/cli/commit/bf15d34d91bead25ccf54ea3002a5dfbd462744d))
* **git-tidy:** add raw-mode checkbox TUI ([bf6bd6a](https://github.com/silee-tools/cli/commit/bf6bd6a9681565f8907b9db67ae6ac2d57cbd980))
* **git-tidy:** add zsh and bash completions ([621256e](https://github.com/silee-tools/cli/commit/621256ed5b9d756d4aae5f081d1cf0d5d3f4b3c5))
* **git-tidy:** show excluded candidates in dry-run output ([0eab9a0](https://github.com/silee-tools/cli/commit/0eab9a0c485c0d09b650584262a31c0df1ef9030))
* **git-tidy:** wire arg parsing, classification, deletion ([156f2c0](https://github.com/silee-tools/cli/commit/156f2c02e641c72099750d4c9191ee9fa3063bdc))


### Refactoring

* **git-tidy:** scaffold Go module, drop zsh plugin ([7f3dc12](https://github.com/silee-tools/cli/commit/7f3dc123f1397ff7e8abc90d9bdfddf9d159f74e))


### Documentation

* **git-tidy:** update repo docs and PRD for Go rewrite ([adbfbc0](https://github.com/silee-tools/cli/commit/adbfbc090af1a8c062646be7d2d7fa4dc6959600))


### Tests

* **git-tidy:** isolate GIT_TIDY_STALE_DAYS env in arg tests ([cc35efb](https://github.com/silee-tools/cli/commit/cc35efb31cb505b785877f95269c4b08702442b9))

## [0.2.1](https://github.com/silee-tools/cli/compare/git-tidy/v0.2.0...git-tidy/v0.2.1) (2026-05-21)


### Documentation

* **plans:** add tool-quality-framework plan and per-tool PRD templates ([7f5692a](https://github.com/silee-tools/cli/commit/7f5692aaecffabfb52aba3d253a8b4d30055870d))
* **prd:** broaden git-tidy definition to protection-rule cleanup model ([fa502b3](https://github.com/silee-tools/cli/commit/fa502b33252c76395ba9b4301489a74c32dcc5ab))
* **prd:** fill in jg/totp/git-tidy PRD bodies from interview ([f32f000](https://github.com/silee-tools/cli/commit/f32f0003b7e13b4437ab6ca466d55966f81c6aad))

## [0.2.0](https://github.com/silee-tools/cli/compare/git-tidy/v0.1.0...git-tidy/v0.2.0) (2026-05-15)


### Features

* unify --version output format across all cli tools ([#61](https://github.com/silee-tools/cli/issues/61)) ([4616876](https://github.com/silee-tools/cli/commit/461687635330c62098f96e56118082a77a3f2eef))

## 0.1.0 (2026-05-15)


### Features

* **git-tidy:** incorporate git-tidy as a zsh-only monorepo tool ([#58](https://github.com/silee-tools/cli/issues/58)) ([93bea94](https://github.com/silee-tools/cli/commit/93bea94a126cd8277b34dbf5ef190c81c02a78e8))
