# Visual design — see docs/design-system.md

This document described "Command Deck": a single dark theme, four
full-strength semantic accents, a three-tier type system with a display
face, and sidebar rows styled as air-traffic-control flight strips.

That's been superseded by a token contract with three selectable themes
(Graphite, Ledger, Command Deck) on a family × mode axis. Command Deck
survives as one option among them, mapped onto the contract rather than
being the contract.

**Read `docs/design-system.md` instead** — it covers the principles, the
full token table with the contrast rules each token is held to, the three
themes, and why mode resolution lives in `theme.ts` rather than a
`prefers-color-scheme` media query.

Source of truth for values: `web/src/styles/tokens.css`.
