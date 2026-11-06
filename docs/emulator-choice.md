# Emulator library choice

**Decision:** use `github.com/charmbracelet/x/vt` behind the `emulator.Emulator` interface.

| Criterion | `charmbracelet/x/vt` | `hinshun/vt10x` |
|---|---|---|
| Maintenance | v0.1.0, last release 2026-10-12 | last release 2022-03-01 (pseudo-version, no tags) |
| Wide characters | Cells carry a grapheme and a width | Not evaluated; rune-based cells |
| Scrollback | Built-in API (`ScrollbackLen`, `ScrollbackCellAt`) | Not evaluated |
| Colors / attributes / hyperlinks | Cell style and OSC 8 link are part of the cell | Not evaluated |

The `x/vt` column comes from its source and the release metadata. The `vt10x` column only reflects release metadata, because its lack of maintenance was enough to rule it out. Re-open this if `x/vt` turns out to mishandle sequences in tickets 02–05; the interface keeps the swap local. No ADR is needed because we did not build our own emulator.
