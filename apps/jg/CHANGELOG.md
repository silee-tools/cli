# Changelog

## [0.7.1](https://github.com/silee-tools/cli/compare/jg/v0.7.0...jg/v0.7.1) (2026-07-11)


### Bug Fixes

* **workflow:** make CLI installation channel deterministic ([afb612e](https://github.com/silee-tools/cli/commit/afb612e4e4a87247bad2e9b445236a79b66d7162))

## [0.7.0](https://github.com/silee-tools/cli/compare/jg/v0.6.0...jg/v0.7.0) (2026-06-11)


### Features

* **jg:** jgw worktree picker 를 이름 중심 표시로 전환 ([#89](https://github.com/silee-tools/cli/issues/89)) ([1c9db27](https://github.com/silee-tools/cli/commit/1c9db276631b6e2ce082e15bbe1316fd1ea9ad08))

## [0.6.0](https://github.com/silee-tools/cli/compare/jg/v0.5.0...jg/v0.6.0) (2026-06-09)


### Features

* **jg:** 무인자 jg 에서 main working tree 를 피커 최상단에 고정 ([#84](https://github.com/silee-tools/cli/issues/84)) ([e35e5f2](https://github.com/silee-tools/cli/commit/e35e5f2baa3e14e7e69e055f7e988111958cab07))

## [0.5.0](https://github.com/silee-tools/cli/compare/jg/v0.4.0...jg/v0.5.0) (2026-05-21)


### Features

* **jg:** add argv0 dispatch and jgw mode skeleton ([357d2d1](https://github.com/silee-tools/cli/commit/357d2d1e74b59b74778fe17ac3892bfd74514cd4))
* **jg:** add jgw zsh and bash completions ([f2bd385](https://github.com/silee-tools/cli/commit/f2bd385a7d7e3a54585507de2402557aea819b0a))
* **jg:** add worktree discovery module ([34d9336](https://github.com/silee-tools/cli/commit/34d9336816ca3ba9e7961a5021b554d06eb32baa))
* **jg:** add worktree frecency store ([18a5a43](https://github.com/silee-tools/cli/commit/18a5a43b93eb98ed402544ff567c954ca8de6774))
* **jg:** add worktree-aware fzf picker helper ([f2ce3f4](https://github.com/silee-tools/cli/commit/f2ce3f4550a3412f4cfcc1853cfbc25887a8c6ad))
* **jg:** emit jgw shell function from jg init ([c566adb](https://github.com/silee-tools/cli/commit/c566adbed8ca4cb7a7509731c61750c1f718ba0e))
* **jg:** implement jgw flow a/b dispatch and body ([5718f53](https://github.com/silee-tools/cli/commit/5718f532fc0947f35080381c0f420de63c19c72c))
* **jg:** install jgw symlink in mise install task ([56c47b4](https://github.com/silee-tools/cli/commit/56c47b4f376bead391126999f203133715364c8c))
* **jg:** omit step counter when worktree picker has a single stage ([812c055](https://github.com/silee-tools/cli/commit/812c055ea60e34441b93db33a3b3747fb42e0822))


### Bug Fixes

* **jg:** harden jgw against empty cwd and missing main worktree ([6591377](https://github.com/silee-tools/cli/commit/6591377e6cc3326d73f3298ca65d02cde772a25f))
* **jg:** identify current worktree across symlinked paths ([2c8648d](https://github.com/silee-tools/cli/commit/2c8648dcb8410d7d29cda12f5f8c2f1f8e15353d))
* **jg:** make preview path resolution POSIX sh compatible ([927f621](https://github.com/silee-tools/cli/commit/927f62121ca8544a9e5b3401b68f3e5f17853e83))
* **jg:** propagate flock error in wtstore.Load ([2d3c6b6](https://github.com/silee-tools/cli/commit/2d3c6b63d0486533b992c5c795da3fafa0709705))
* **jg:** stop double-quoting fzf placeholder in preview commands ([8e43e32](https://github.com/silee-tools/cli/commit/8e43e322d1b6296bcd79f02fdd457d386c35599c))


### Refactoring

* **jg:** move repo store to XDG_STATE_HOME with auto-migration ([88f7468](https://github.com/silee-tools/cli/commit/88f74684aec1a03af210ecfb294fa92f9ef9a6fa))
* **jg:** single-source shell integration via go:embed ([8d445b2](https://github.com/silee-tools/cli/commit/8d445b2200ba8b131b0b0115e6a71e1629f45805))


### Documentation

* **plans:** add tool-quality-framework plan and per-tool PRD templates ([7f5692a](https://github.com/silee-tools/cli/commit/7f5692aaecffabfb52aba3d253a8b4d30055870d))
* **prd:** fill in jg/totp/git-tidy PRD bodies from interview ([f32f000](https://github.com/silee-tools/cli/commit/f32f0003b7e13b4437ab6ca466d55966f81c6aad))


### Tests

* **jg:** harden shell init tests to catch jgw body regressions ([e45540a](https://github.com/silee-tools/cli/commit/e45540a417bf98d2cddb0758950c30792954c236))
* **jg:** lock XDG-over-legacy precedence and clean test stub ([16fc00a](https://github.com/silee-tools/cli/commit/16fc00ae5fb5f1f31d38c2ce78a882d8d904b504))
* **jg:** reset LegacyDataFile in entry test helper ([e3fe9ca](https://github.com/silee-tools/cli/commit/e3fe9ca4706841348d1359a053fb45502c685edb))

## [0.4.0](https://github.com/silee-tools/cli/compare/jg/v0.3.1...jg/v0.4.0) (2026-05-15)


### Features

* unify --version output format across all cli tools ([#61](https://github.com/silee-tools/cli/issues/61)) ([4616876](https://github.com/silee-tools/cli/commit/461687635330c62098f96e56118082a77a3f2eef))

## [0.3.1](https://github.com/silee-tools/cli/compare/jg/v0.3.0...jg/v0.3.1) (2026-05-15)


### Documentation

* document fmt-check commands ([#50](https://github.com/silee-tools/cli/issues/50)) ([5b4ef19](https://github.com/silee-tools/cli/commit/5b4ef199938db0e8e7a99f81192cdf56d119908c))

## [0.3.0](https://github.com/silee-tools/cli/compare/jg/v0.2.2...jg/v0.3.0) (2026-05-14)


### Features

* add jg clean scheduler ([#45](https://github.com/silee-tools/cli/issues/45)) ([0280a03](https://github.com/silee-tools/cli/commit/0280a032c02a9e594300e5440d43488f28b07123))


### Documentation

* clarify jg shell setup ([#43](https://github.com/silee-tools/cli/issues/43)) ([cfe3d33](https://github.com/silee-tools/cli/commit/cfe3d33fee2d8c52fa6cefe434189a628f52d520))

## [0.2.2](https://github.com/silee-tools/cli/compare/jg/v0.2.1...jg/v0.2.2) (2026-05-11)


### Bug Fixes

* **jg:** dispatch release rebuild workflow correctly ([#23](https://github.com/silee-tools/cli/issues/23)) ([5b06ae3](https://github.com/silee-tools/cli/commit/5b06ae3ca401bb551a7565cc0b1245a891adc4fc))


### CI

* update goreleaser archive format metadata ([#31](https://github.com/silee-tools/cli/issues/31)) ([a1d5ea5](https://github.com/silee-tools/cli/commit/a1d5ea5493b2fc33be2484d32fe7a2d7ba287af2))

## [0.2.1](https://github.com/silee-tools/cli/compare/jg/v0.2.0...jg/v0.2.1) (2026-05-09)


### Bug Fixes

* **release:** GoReleaser monorepo 블록 제거 (Pro 전용) + workflow_dispatch 추가 ([eb70b2b](https://github.com/silee-tools/cli/commit/eb70b2b196d98a9e13700825181765b7aa40eb00))

## [0.2.0](https://github.com/silee-tools/cli/compare/jg/v0.1.27...jg/v0.2.0) (2026-05-09)


### Features

* **release:** GoReleaser/bun/bash 분기 release.yml + 도구별 .goreleaser.yaml ([6807962](https://github.com/silee-tools/cli/commit/68079628aee83fb88294555281c6e2319e79a49c))
* **release:** release-please + GoReleaser 정석 패턴으로 마이그레이션 ([27b9fc7](https://github.com/silee-tools/cli/commit/27b9fc785edb39d22f1f7f154cdd409cdae45631))
* 기존 5개 도구를 apps/ 하위로 이주 ([27d4ffe](https://github.com/silee-tools/cli/commit/27d4ffe9bf10736ef6808142b154b1aaca5ead5d))


### Bug Fixes

* **ci:** bats 도구 추가 + errcheck 전체 비활성화 ([48fd55f](https://github.com/silee-tools/cli/commit/48fd55f52aa926bef9b7759ac03fdb1efc03e780))
* **ci:** lint config 추가 — 원본 레포 implicit 정책 매칭 ([8a6156e](https://github.com/silee-tools/cli/commit/8a6156e39eaea76b928f148dc4452f55ec0d8465))
* **ci:** mise-action working_directory + gofmt/shfmt 일괄 적용 ([a716695](https://github.com/silee-tools/cli/commit/a71669532da90f081dbffbcb07d6bd6fcb9bc751))
* **ci:** shfmt -i 2 + golangci-lint 도구 등록 + bun test 빈 케이스 처리 ([c1f811a](https://github.com/silee-tools/cli/commit/c1f811ab862b9c6e7a6e4d24960ca14432d179d5))
