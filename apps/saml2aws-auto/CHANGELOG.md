# Changelog

## [2.0.8](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.7...saml2aws-auto/v2.0.8) (2026-05-15)


### Documentation

* document fmt-check commands ([#50](https://github.com/silee-tools/cli/issues/50)) ([5b4ef19](https://github.com/silee-tools/cli/commit/5b4ef199938db0e8e7a99f81192cdf56d119908c))

## [2.0.7](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.6...saml2aws-auto/v2.0.7) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** respect configured session duration ([f16ca48](https://github.com/silee-tools/cli/commit/f16ca48516165b6e3715869194dd5b1efa959a42))

## [2.0.6](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.5...saml2aws-auto/v2.0.6) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** force twelve hour sessions ([397f9c9](https://github.com/silee-tools/cli/commit/397f9c95e58a57e2c59e0b135f987d06a0654ea6))

## [2.0.5](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.4...saml2aws-auto/v2.0.5) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** remove zsh status wrapper ([0f66f1c](https://github.com/silee-tools/cli/commit/0f66f1c89293b0c052b825e8ce471768827e444b))

## [2.0.4](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.3...saml2aws-auto/v2.0.4) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** restore aws prompt status ([38e506f](https://github.com/silee-tools/cli/commit/38e506f4441256d5b06c6d00787f0cd2b6ea3bb4))

## [2.0.3](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.2...saml2aws-auto/v2.0.3) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** derive zsh plugin path from command ([d13b990](https://github.com/silee-tools/cli/commit/d13b9906c9522434bf40f6f9557cc2810ab10cb2))

## [2.0.2](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.1...saml2aws-auto/v2.0.2) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** print direct zsh init snippet ([eef2346](https://github.com/silee-tools/cli/commit/eef2346a4ecd06fa75e60cc96156d9cbaccdab7f))

## [2.0.1](https://github.com/silee-tools/cli/compare/saml2aws-auto/v2.0.0...saml2aws-auto/v2.0.1) (2026-05-11)


### Bug Fixes

* **saml2aws-auto:** print installed zsh plugin path ([8b2da46](https://github.com/silee-tools/cli/commit/8b2da4689c02691c61028b8581e299c6d2991458))

## [2.0.0](https://github.com/silee-tools/cli/compare/saml2aws-auto/v1.0.1...saml2aws-auto/v2.0.0) (2026-05-11)


### ⚠ BREAKING CHANGES

* **saml2aws-auto:** saml2aws-auto-login is removed. Use saml2aws-auto login/check/init zsh instead.

### Features

* **saml2aws-auto:** replace login wrapper with unified CLI ([813af4d](https://github.com/silee-tools/cli/commit/813af4d70ef9302f70c6bf9ede65dbc7cbb0bd05))


### Bug Fixes

* **jg:** dispatch release rebuild workflow correctly ([#23](https://github.com/silee-tools/cli/issues/23)) ([5b06ae3](https://github.com/silee-tools/cli/commit/5b06ae3ca401bb551a7565cc0b1245a891adc4fc))


### CI

* update goreleaser archive format metadata ([#31](https://github.com/silee-tools/cli/issues/31)) ([a1d5ea5](https://github.com/silee-tools/cli/commit/a1d5ea5493b2fc33be2484d32fe7a2d7ba287af2))

## [1.0.1](https://github.com/silee-tools/cli/compare/saml2aws-auto/v1.0.0...saml2aws-auto/v1.0.1) (2026-05-09)


### Bug Fixes

* **release:** GoReleaser monorepo 블록 제거 (Pro 전용) + workflow_dispatch 추가 ([eb70b2b](https://github.com/silee-tools/cli/commit/eb70b2b196d98a9e13700825181765b7aa40eb00))

## 1.0.0 (2026-05-09)


### Features

* **release:** release-please + GoReleaser 정석 패턴으로 마이그레이션 ([27b9fc7](https://github.com/silee-tools/cli/commit/27b9fc785edb39d22f1f7f154cdd409cdae45631))
* totp / saml2aws-auto Go CLI 신규 작성 (zsh 함수 → standalone) ([958becf](https://github.com/silee-tools/cli/commit/958becf203aca88a2e46c13efd5d21249fa9a7a1))
