# Changelog

## Unreleased

- Removed **Explain this finding**. It opened the raw evidence trace, which
  is a debugging dump and not useful to read.
- `Tab` and `Shift+Tab` move between the steps of a path.
- The status bar is shorter. Details such as the time of the last check are
  in its tooltip.

## 0.8.0

First release.

- Findings appear shortly after you stop typing, including in unsaved files,
  with the steps that lead to each one.
- Hover a finding for the usual fix. Step through its path from the lightbulb
  menu, or open the full evidence with **Explain this finding**.
- The gohawk binary for your platform ships with the extension.
- The status bar shows what the last check found, and says so when a package
  does not build or a check was stopped.
