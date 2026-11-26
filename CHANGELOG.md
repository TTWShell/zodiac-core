# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2026-11-26

### Added

- Generated services include an in-memory user module at `internal/user`, with `GET /users` and `GET /users/{id}`.
- `humatransport.NewError` and `humatransport.NewErrorWithContext` integrate Huma framework errors with Zodiac responses and request logging.
- Usage guides cover XML requests and raw XML, HTML, CSV, text, and binary responses. A WebSocket example demonstrates message handling and shutdown, including upgrades in progress.

### Changed

- Generated services place the command in `cmd/<name>` and compose feature Fx modules from `internal/app`. HTTP setup lives in `internal/httpserver`; run newly generated services with `go run ./cmd/<name> serve`.
- Huma errors use the `code`/`data`/`message` envelope, with client error details in `data.errors`. Generated services install the error factories at startup; existing applications must assign `huma.NewError = humatransport.NewError` and `huma.NewErrorWithContext = humatransport.NewErrorWithContext` before creating APIs to apply this format to framework errors. These factories affect all Huma APIs in the process.
- Error documentation follows Huma's native `Errors` and `Responses` rules; an entry in `Errors` overrides `Responses` for the same status.

### Fixed

- Error normalization preserves response headers carried by wrapped errors and logs internal error causes without including those causes in responses.

## [0.1.0] - 2026-11-26

Initial release. Requires Go 1.26.0 or newer.

### Added

- `zodiac new` generates standalone services with Cobra commands, Fx dependency injection, chi/Huma APIs, a health endpoint, and graceful shutdown.
- Layered YAML configuration supports environment selection, configurable search paths, explicit environment variable bindings, strict struct decoding, and application validation.
- Generic `code`/`data`/`message` responses and public business errors integrate with Huma and OpenAPI while preserving Huma's native validation responses.
- HTTP middleware provides request IDs, access logs, panic recovery, and request metadata shared through context.
- JSON logging uses `slog` with configurable levels and request and service metadata.
- Usage guides, versioned documentation, and CI checks cover the library and generated services across Go 1.26 and 1.27.

[Unreleased]: https://github.com/TTWShell/zodiac-core/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/TTWShell/zodiac-core/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/TTWShell/zodiac-core/releases/tag/v0.1.0
