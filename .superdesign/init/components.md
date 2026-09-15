# Components

The desktop frontend currently has no shared component library. UI is composed inline in `desktop-app/frontend/src/main.tsx` with styles in `desktop-app/frontend/src/style.css`.

## Inline primitives
- Buttons: native `<button>` elements styled by `style.css`
- Text input: native `<textarea>`
- Panels/cards: native `<section>` and `<article>` elements with `.panel` / `.grid`
- Status badge: `.pill`
