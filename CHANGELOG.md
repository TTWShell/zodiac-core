# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-11-26

Initial release. Requires Go 1.26.0 or newer.

### Added

- `zodiac new` generates standalone services with Cobra commands, Fx dependency injection, chi/Huma APIs, a health endpoint, and graceful shutdown.
- Layered YAML configuration supports environment selection, configurable search paths, explicit environment variable bindings, strict struct decoding, and application validation.
- Generic `code`/`data`/`message` responses and public business errors integrate with Huma and OpenAPI while preserving Huma's native validation responses.
- HTTP middleware provides request IDs, access logs, panic recovery, and request metadata shared through context.
- JSON logging uses `slog` with configurable levels and request and service metadata.
- Usage guides, versioned documentation, and CI checks cover the library and generated services across Go 1.26 and 1.27.

[Unreleased]: https://github.com/TTWShell/zodiac-core/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/TTWShell/zodiac-core/releases/tag/v0.1.0
