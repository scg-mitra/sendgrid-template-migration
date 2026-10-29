# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and versions follow
[Semantic Versioning](https://semver.org/).

The section for a version becomes the body of its GitHub Release, so write
entries for the people using the app. Group them under these headings and
leave out any that are empty: Added, Changed, Fixed, Removed, Security.

## [Unreleased]

### Added

- Choose what to do with each template whose name already exists in the
  destination: skip it, add its versions to the existing template, create a
  duplicate, or copy it under a new name (a free name is suggested). "Set all
  to" applies one choice to every clash, and the list scrolls when it's long.

## [1.2.1] - 2026-10-29

### Changed

- The copy dialog's rename option is now "Add to each name", with a
  "Template name" marker between the two boxes and a preview of the new name,
  so it's clear the text is added to the existing name rather than replacing
  it.
- When a name already exists in the destination, the copy dialog says which
  templates clash and suggests adding text to the names as another way round
  it. It confirms when the new names avoid every clash.

## [1.2.0] - 2026-10-29

### Changed

- The window opens larger (1040×720), and the Connect screen shows "Copy
  from" and "Copy to" side by side, so everything fits without scrolling.
- Only long lists scroll: the copy results list now scrolls on its own,
  keeping the progress bar and actions in view.
- Shorter labels on the source and destination buttons ("API key",
  "Subuser", "Parent").
- New README screenshots, including the Connect screen.

## [1.1.2] - 2026-10-29

### Fixed

- Release builds no longer show warnings about GitHub Actions running on the
  deprecated Node.js 20, or frontend lint warnings.

## [1.1.1] - 2026-10-29

### Changed

- Release notes now list what was added, changed and fixed in each version.

## [1.1.0] - 2026-10-29

### Added

- Copy from a subuser, to the parent account, or between subusers.
- Export templates to a backup file and copy from that file later.
- Choose what happens when a template name already exists in the
  destination: skip it, add the versions to the existing template, or create
  a duplicate.
- Rename copies with a prefix and suffix.
- Copy up to 4 templates at once, and cancel or retry from the Copy screen.
- Select-all checkbox, shift-click to select ranges, type and "hide existing"
  filters, and a template preview.
- Copy options dialog, CSV export of results, and a clickable stepper.
- A notice in the header when a newer release is available.
- Command-line flags: `-list`, `-dry-run`, `-json`, `-export`, `-import`,
  `-src-subuser`, `-prefix`, `-suffix` and `-on-existing`.

### Changed

- The data region is now the first field, with a hint for API keys that don't
  start with `SG.`.
- "Start over" replaces quitting at the end of a copy.

### Fixed

- A failed or cancelled copy no longer leaves a partial template (or partly
  added versions) in the destination.
- GET and DELETE requests are retried after 5xx responses and network errors.

## [1.0.0] - 2026-10-28

### Added

- Desktop app for Windows, macOS and Linux that copies SendGrid templates,
  with all their versions, from a parent account to a subuser, using either
  the subuser's API key or the parent key acting on behalf of the subuser.
- Connect, choose templates and copy with live progress.
- Command-line mode (`-template`) for scripted copies.
- Rate-limit retries and plain-language error messages.

[Unreleased]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.1.2...v1.2.0
[1.1.2]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.1.1...v1.1.2
[1.1.1]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.1.0...v1.1.1
[1.1.0]: https://github.com/scg-mitra/sendgrid-template-migration/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/scg-mitra/sendgrid-template-migration/releases/tag/v1.0.0
