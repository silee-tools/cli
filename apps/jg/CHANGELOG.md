# Changelog

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
