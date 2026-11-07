# ssterm

A CLI that turns terminal output into a faithful, shareable image of what a terminal would have shown.

## Language

**Screenshot**:
A rendered image (SVG, optionally exported to PNG) of terminal output as a terminal would display it.
_Avoid_: Capture, snapshot, export

**Source**:
Where the terminal output comes from: either piped stdin or a command run by ssterm inside a pseudo-terminal.
_Avoid_: Input, feed

**Screen grid**:
The 2D grid of cells produced by emulating the output, each cell holding a character and its styling.
_Avoid_: Buffer, canvas

**Theme**:
A pluggable look for a specific terminal: color palette and window frame. Independent of how output is parsed.
_Avoid_: Skin, style, preset

**Chrome**:
The window decoration (title bar, buttons, optional title) drawn around the screen grid. Its look is defined by the Theme; showing it is an Enhancement, off by default.
_Avoid_: Frame, window decoration

**Scrollback**:
Lines that have scrolled off the top of the screen grid. Included in a Screenshot by default.
_Avoid_: History

**Faithful**:
The default rendering mode: the screen grid is shown exactly as emulated, with no cosmetic changes.
_Avoid_: Raw, as-is

**Enhancement**:
An opt-in cosmetic change to a screenshot, such as trimming, cropping, padding, window chrome or shadow.
_Avoid_: Improvement, beautify, filter
