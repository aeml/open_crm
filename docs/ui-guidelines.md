# UI Guidelines

Visual and interaction conventions for Open CRM. The goal is a consistent, readable, low-noise UI built from a small set of design primitives. These guidelines reflect decisions already made in the codebase, not aspirational plans.

## Design stance

Nice UI does not require dependency obesity. Every visual decision should come from deliberate taste, not from a component library's defaults.

- Neutral base palette with one strong accent color.
- Generous whitespace.
- Strong typography contrast between headings and body.
- Cards and panels used sparingly, not as a wrapper for everything.
- Inline feedback close to the field or action that caused it.
- No glass morphism, no heavy gradients, no animation for its own sake.

---

## Color tokens (`src/styles/tokens.css`)

| Token | Value | Use |
|---|---|---|
| `--bg` | `#f5f7fb` | Page background |
| `--surface` | `#ffffff` | Card, modal, dropdown backgrounds |
| `--surface-muted` | `#f9fbff` | Subtler surface (table row hover, sidebar) |
| `--text` | `#152033` | Primary body text |
| `--text-muted` | `#5f6f86` | Labels, hints, secondary info |
| `--border` | `#d9e1ec` | Dividers, input borders, card borders |
| `--accent` | `#275efe` | Primary action color (buttons, links, active states) |
| `--accent-strong` | `#1745cc` | Accent hover/pressed state |
| `--shadow` | `0 18px 40px rgba(19,35,66,0.08)` | Card elevation shadow |

**Rules:**
- Do not introduce new named colors outside `tokens.css`.
- Danger states use `#b42318` (hardcoded; add a `--danger` token if it appears in more than three places).
- Do not use raw hex values in component CSS; use tokens.

---

## Spacing scale

The spacing scale is `0.5rem` increments via tokens.

| Token | Value | Typical use |
|---|---|---|
| `--space-1` | `0.5rem` (8px) | Icon gaps, tight inline spacing |
| `--space-2` | `0.75rem` (12px) | Button padding, form element gaps |
| `--space-3` | `1rem` (16px) | Default block spacing |
| `--space-4` | `1.5rem` (24px) | Card padding, section gaps |
| `--space-5` | `2rem` (32px) | Page section separations |
| `--space-6` | `3rem` (48px) | Large layout gaps |

Use the scale. Do not invent arbitrary pixel values for spacing.

---

## Border radius

| Token | Value | Use |
|---|---|---|
| `--radius-sm` | `12px` | Small interactive elements (chips, tags) |
| `--radius-md` | `18px` | Cards, panels |
| `--radius-lg` | `24px` | Modals, large containers |

Buttons use `border-radius: 999px` (full pill) by convention.

---

## Typography

Font: `Inter, ui-sans-serif, system-ui, -apple-system, …`

- Page headings (`h1`, `h2`): set `margin: 0`; use `--space-*` for separation.
- Eyebrow labels: `.eyebrow` class — `0.82rem`, `700` weight, uppercase, `0.08em` letter-spacing, `var(--accent)` color.
- Metric values: `1.8rem`, stand-alone count display.
- Muted/secondary text: `var(--text-muted)` — hints, labels, secondary info.

Never use `font-size` below `0.8rem` for anything meant to be read.

---

## UI primitives (`src/components/ui/`)

### `Button`

```jsx
<Button>Label</Button>                          // primary (default)
<Button className="button-secondary">Label</Button>
<Button className="button-ghost">Cancel</Button>
<Button className="button-danger">Delete</Button>
```

- Always use the `Button` component for interactive actions; do not write raw `<button>` in route files.
- Destructive actions use `.button-danger`.
- Cancel / low-emphasis actions use `.button-ghost` or `.button-secondary`.
- Disabled state: pass `disabled` prop; do not hide buttons to indicate unavailability.

### `Card`

```jsx
<Card>content</Card>
<Card className="custom-modifier">content</Card>
```

- Renders a `<section>` with `.card` styles (white surface, border, radius, shadow, padding).
- Use for discrete data groups on detail pages and dashboard panels.
- Do not nest cards.

### `Field`

```jsx
<Field label="Email" hint="We'll never share this.">
  <input type="email" name="email" />
</Field>
```

- Wraps label, input, and optional hint text.
- Always use for form inputs; do not float bare labels next to inputs.
- Hint renders muted below the input.

### `EmptyState`

```jsx
<EmptyState
  icon="👥"
  title="No contacts yet"
  description="Add your first contact to get started."
  action={<Button onClick={...}>Add contact</Button>}
/>
```

- Use on any list or detail section that can be empty.
- Provide a `title`, `description`, and an `action` when there is a clear next step.
- Do not render an empty `<ul>` or blank panel; empty states are intentional.

---

## Layout primitives (`src/components/layout/`)

### `AppHeader`

Top navigation bar. Contains the app name and the active org context. Rendered by the root shell; do not render it inside route components.

### `SideNav`

Left navigation with links to main sections. Rendered by the root shell. Active link is derived from the current route path.

### `PageHeader`

```jsx
<PageHeader title="Contacts" description="optional subtitle">
  <Button>Add contact</Button>
</PageHeader>
```

- Renders page title, optional description, and optional right-slot for page actions.
- Use at the top of every route component that represents a top-level page.

---

## Form patterns

- Validate on submit, not on every keystroke, unless the field has hard constraints (e.g., character limits).
- Show field-level error messages below the relevant `Field`, not as a banner.
- A banner error is appropriate for errors not attributable to a specific field (e.g., "Server error, please try again").
- Use `required` on `<input>` elements for basic browser-native hint; do not rely on it for actual validation.
- Submit buttons should be disabled or show a loading state while the request is in flight.

---

## Table and list patterns

- Paginated lists use `page` / `pageSize` query params; default page size is 25.
- Search uses a controlled input with debounce before the API call.
- Row links navigate to detail pages; the entire row is not an anchor (keyboard and screen reader friendliness).
- Archived records are excluded by default; show them only with an explicit filter.
- Empty filtered results show a different empty state than an empty account (reset/clear action vs. add action).

---

## Copy and tone

- Use plain imperative labels for actions: "Add contact", "Save changes", "Archive deal".
- Avoid "Submit" as a button label; use the specific action.
- Confirmation copy for destructive actions should state what will happen: "Archive this contact?" not "Are you sure?".
- Error messages name the problem and suggest a fix where possible.
- Avoid engineering jargon in user-visible strings ("400 Bad Request", "null reference", etc.).

---

## CSS organization

| File | Purpose |
|---|---|
| `tokens.css` | Design tokens (colors, spacing, radius, shadow, font) |
| `base.css` | HTML reset and element defaults |
| `layout.css` | Page shell, grid, flex utilities |
| `components.css` | Shared component class styles (`.card`, `.button`, `.field`, `.table`, etc.) |

- Add new component styles to `components.css`, scoped by class name.
- Do not use inline styles for anything that will appear in more than one place.
- Do not import CSS inside component files (Vite processes `main.jsx` → `styles/` import chain).
