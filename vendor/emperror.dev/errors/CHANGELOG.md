# Change Log


All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](http://keepachangelog.com/en/1.0.0/)
and this project adheres to [Semantic Versioning](http://semver.org/spec/v2.0.0.html).


## [Unreleased]


## [0.8.1] - 2022-02-23

### Fixed

- Reduced memory allocation when collecting stack information


## [0.8.0] - 2020-09-14

### Changed

- Update dependencies


## [0.7.0] - 2020-01-13

### Changed

- Updated dependencies ([github.com/pkg/errors](https://github.com/pkg/errors))
- Use `WithMessage` and `WithMessagef` from [github.com/pkg/errors](https://github.com/pkg/errors)


## [0.6.0] - 2020-01-09

### Changed

- Updated dependencies


## [0.5.2] - 2020-01-06

### Changed

- `match`: exported `ErrorMatcherFunc`


## [0.5.1] - 2020-01-06

### Fixed

- `match`: race condition in `As`


## [0.5.0] - 2020-01-06

### Added

- `match` package for matching errors


## [0.4.3] - 2019-09-05

### Added

- `Sentinel` error type for creating [constant error](https://dave.cheney.net/2016/04/07/constant-errors)


## [0.4.2] - 2019-07-19

### Added

- `NewWithDetails` function to create a new error with details attached


## [0.4.1] - 2019-07-17

### Added

- `utils/keyval` package to work with key-value pairs.


## [0.4.0] - 2019-07-17

### Added

- Error details


## [0.3.0] - 2019-07-14

### Added

- Multi error
- `UnwrapEach` function


## [0.2.0] - 2019-07-12

### Added

- `*If` functions that only annotate an error with a stack trace if there isn't one already in the error chain


## [0.1.0] - 2019-07-12

- Initial release


[Unreleased]: https://github.com/emperror/errors/compare/v0.8.1...HEAD
[0.8.1]: https://github.com/emperror/errors/compare/v0.8.0...v0.8.1
[0.8.0]: https://github.com/emperror/errors/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/emperror/errors/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/emperror/errors/compare/v0.5.2...v0.6.0
[0.5.2]: https://github.com/emperror/errors/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/emperror/errors/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/emperror/errors/compare/v0.4.3...v0.5.0
[0.4.3]: https://github.com/emperror/errors/compare/v0.4.2...v0.4.3
[0.4.2]: https://github.com/emperror/errors/compare/v0.4.1...v0.4.2
[0.4.1]: https://github.com/emperror/errors/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/emperror/errors/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/emperror/errors/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/emperror/errors/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/emperror/errors/compare/v0.0.0...v0.1.0
