# 0001. Web design system: tokens, neutral dark theme, system fonts, logo variants

**Status:** accepted (Phase 0)

## Context
The UI should feel like LeetCode (dense, calm, utilitarian) without copying its logo, colors, or copy. The project owner wants LeetCode's black/grey dark background and system fonts, a blue brand theme, and a logo that stays visible in both themes.

## Decision
- **Tokens only.** All colors are CSS variables in `web/src/app/globals.css` (`--lf-*`). Components use semantic Tailwind aliases (`bg-panel`, `text-muted`, `bg-primary`, ...), never raw hex.
- **Neutral dark theme.** Dark mode surfaces use a black/grey ramp (`--lf-ink-950/900/800/700` = `#0f0f0f/#1a1a1a/#262626/#3a3a3a`). Navy tokens stay for light-mode text and brand accents.
- **Theme switching.** Follows `prefers-color-scheme`; the nav toggle sets `data-theme` on `<html>` and stores `lf-theme` in `localStorage`. A tiny inline script applies the stored value before paint.
- **Fonts.** LeetCode's system stack for UI and `Menlo, Monaco, Consolas, "Courier New"` for code. No web fonts are downloaded.
- **Logo.** `logo-mark.svg` (no background) in the navbar and favicon; `logo-mark-light.svg` is the same file with only the centre bar filled `#3a3a3a`, shown in light theme; `logo.svg` (black tile) for README/OG.

## Alternatives
- Keep navy dark theme (original spec): on-brand but not LeetCode-like; rejected by the owner.
- Inter + JetBrains Mono via `next/font`: consistent across OSes but adds font downloads and differs from LeetCode.
- Recolor the logo bar with CSS: impossible when the SVG is loaded through `<img>`, so a second static file is used.

## Consequences
- Adding a color means adding a token first; lint-by-review enforces this (no automated check yet).
- The favicon cannot follow the page theme, so it keeps the cream-bar mark.
- Text renders in a different font per OS (Segoe UI on Windows, SF on macOS).
