# SilkStrand Design System

**Version:** 0.1 (draft)
**Status:** Spec. Current UIs **do not** match yet — migration planned.
**Applies to:** `web/` (tenant frontend) and `backoffice/web/` (admin frontend).

---

## Why this doc exists

Two frontends with duplicated CSS are drifting apart. One shared system — tokens, components, copy rules — keeps them coherent as we add pages. It also forces us to answer questions we'd otherwise answer inconsistently (destructive-action copy, empty-state icons, form validation timing).

This is v0.1. Expect revisions. If something here conflicts with a shipped screen, the screen is wrong — open an issue or fix it.

---

## 1. Brand

### 1.1 Personality

SilkStrand should feel:

- Lightweight
- Secure
- Technical but approachable
- Enterprise-ready
- Calm and precise
- **"Quietly powerful"**

Avoid:

- Loud neon security colors
- Hacker-style green terminals
- Playful/jokey UI
- Heavy gradients
- Visual clutter
- Padlocks, flames, shields (the security-product cliché set)

### 1.2 Name

Always one word, Pascal-case: **SilkStrand**. Not "Silk Strand", not "silkstrand" in prose. Product surfaces and docs use SilkStrand; CLI/binary/DNS/system names use `silkstrand` (lowercase).

### 1.3 Logo direction

Committed direction: **woven-thread mark**. Three to five strands converging diagonally to suggest a secure fiber/tunnel. Monochrome, outline-friendly, works at 16px (favicon) and 200px (login). No shield, no lock, no flame.

Asset work is a follow-up; until we have a final mark, plain wordmark (`SilkStrand`) in `--ss-text-primary` at the appropriate weight is the interim logo.

---

## 2. Theme: light, dark, and system

SilkStrand ships **light + dark themes with system-preference detection by default**. Users can override via a persisted setting.

### 2.1 Policy

- System preference (`prefers-color-scheme`) is the default.
- User override is stored in `localStorage` as `silkstrand_theme` with values `"light"` | `"dark"` | `"system"`.
- Theme is applied by setting `data-theme="light"` or `data-theme="dark"` on `<html>` (never on `<body>` — avoids a flash if `body` mounts slowly).
- Before React mounts, a tiny inline `<script>` in `index.html` reads the preference and applies the attribute. Prevents a flash of the wrong theme on load.
- Components never hard-code colors; always read from tokens.

### 2.2 Accessibility floor

- **WCAG AA minimum** for body text (`4.5:1`). Large text (`3:1`) acceptable for display type only.
- **Focus-visible ring** must be visible on all interactive elements. Default ring: 2px `--ss-accent-primary`, 2px outset from the element.
- **Never use color alone** to convey meaning. Status badges pair color with a label word. Destructive buttons use color + the word "Delete/Remove/Suspend".
- **Keyboard navigation:** tab order follows visual order; no positive `tabindex` values.
- **Reduced motion:** respect `prefers-reduced-motion: reduce` — disable all `transform`/`translate` transitions.

---

## 3. Tokens

Tokens are defined as CSS custom properties on `:root[data-theme="light"]` and `:root[data-theme="dark"]`. A shared `packages/design-tokens/tokens.css` (or equivalent) is the single source of truth; both frontends import it.

### 3.1 Color

Naming rule: tokens describe **role**, never raw color. `--ss-bg-surface`, not `--ss-slate-900`.

#### Shared tokens (both themes)

```css
/* Accent — brand blue. Identical across themes. */
--ss-accent-primary: #3b82f6;
--ss-accent-hover:   #2563eb;
--ss-accent-subtle:  #1d4ed8;
--ss-accent-soft:    #93c5fd;

/* Status. Same hue in both themes; background opacity differs. */
--ss-success: #10b981;
--ss-warning: #f59e0b;
--ss-danger:  #ef4444;
--ss-info:    #06b6d4;
```

#### Dark theme

```css
:root[data-theme="dark"] {
  --ss-bg-base:      #0f172a;  /* page background */
  --ss-bg-surface:   #1f2937;  /* cards, panels */
  --ss-bg-subtle:    #111827;  /* sidebar, muted panels */
  --ss-bg-raised:    #374151;  /* hovered rows, raised surfaces */

  --ss-text-primary:   #f9fafb;
  --ss-text-secondary: #d1d5db;
  --ss-text-muted:     #9ca3af;
  --ss-text-on-accent: #ffffff;

  --ss-border-subtle:  #1f2937;
  --ss-border-default: #374151;
  --ss-border-strong:  #4b5563;

  /* Status backgrounds — low-opacity overlays on dark. */
  --ss-success-bg: rgba(16,185,129,.15);
  --ss-warning-bg: rgba(245,158,11,.15);
  --ss-danger-bg:  rgba(239,68,68,.15);
  --ss-info-bg:    rgba(6,182,212,.15);
}
```

#### Light theme

```css
:root[data-theme="light"] {
  --ss-bg-base:      #ffffff;
  --ss-bg-surface:   #ffffff;
  --ss-bg-subtle:    #f9fafb;
  --ss-bg-raised:    #f3f4f6;

  --ss-text-primary:   #111827;
  --ss-text-secondary: #374151;
  --ss-text-muted:     #6b7280;
  --ss-text-on-accent: #ffffff;

  --ss-border-subtle:  #e5e7eb;
  --ss-border-default: #d1d5db;
  --ss-border-strong:  #9ca3af;

  /* Status backgrounds — paler on light. */
  --ss-success-bg: #d1fae5;
  --ss-warning-bg: #fef3c7;
  --ss-danger-bg:  #fee2e2;
  --ss-info-bg:    #cffafe;
}
```

### 3.2 Spacing (4px grid)

```css
--ss-space-xs:  4px;
--ss-space-sm:  8px;
--ss-space-md:  12px;
--ss-space-lg:  16px;   /* default inside cards */
--ss-space-xl:  24px;   /* between sections */
--ss-space-2xl: 32px;
--ss-space-3xl: 48px;   /* between top-level page sections */
```

Rule: any margin/padding value used in a component must come from this scale. No `margin: 13px`.

### 3.3 Radius

```css
--ss-radius-sm: 4px;   /* inputs, small buttons */
--ss-radius-md: 6px;   /* primary buttons */
--ss-radius-lg: 8px;   /* cards, modals */
--ss-radius-full: 999px; /* pills, avatars */
```

### 3.4 Shadow

```css
--ss-shadow-sm: 0 1px 2px rgba(0,0,0,.05);
--ss-shadow-md: 0 4px 6px rgba(0,0,0,.07);
--ss-shadow-lg: 0 10px 15px rgba(0,0,0,.10);
```

Use sparingly in light mode; almost never in dark mode (shadow on dark is visually weak — prefer a border highlight).

### 3.5 Motion

```css
--ss-transition-fast: 120ms ease;
--ss-transition-base: 180ms ease;
```

Hover transitions only on `background-color`, `color`, `border-color`, `opacity`. Never animate layout properties on interactive states (height, width). Respect `prefers-reduced-motion: reduce`.

---

## 4. Typography

### 4.1 Stack

```css
font-family:
  Inter,
  system-ui,
  -apple-system,
  "Segoe UI",
  Roboto,
  sans-serif;
```

Monospace (code, IDs, command lines):

```css
font-family:
  "JetBrains Mono",
  ui-monospace,
  "SF Mono",
  Menlo,
  Consolas,
  monospace;
```

### 4.2 Sizes (semantic, not shirt-sized)

```css
--ss-text-caption: 12px;   /* table headers, help text */
--ss-text-body-sm: 13px;
--ss-text-body:    14px;   /* default */
--ss-text-body-lg: 16px;   /* prose / marketing */
--ss-text-h4:      16px;
--ss-text-h3:      18px;
--ss-text-h2:      22px;
--ss-text-h1:      28px;
```

### 4.3 Weight

```css
--ss-font-regular:  400;
--ss-font-medium:   500;
--ss-font-semibold: 600;
--ss-font-bold:     700;
```

Headings use `--ss-font-semibold`. Body is `--ss-font-regular`. Buttons use `--ss-font-medium`.

### 4.4 Line height

```css
--ss-leading-tight:  1.2;   /* headings */
--ss-leading-normal: 1.5;   /* body */
--ss-leading-loose:  1.7;   /* long-form prose, docs */
```

---

## 5. Components

### 5.1 Buttons

Three variants: **primary**, **secondary**, **danger**. Plus `btn-sm` / `btn-md` (default) size modifiers.

```css
.ss-btn {
  display: inline-flex;
  align-items: center;
  gap: var(--ss-space-sm);
  padding: 8px 14px;
  border-radius: var(--ss-radius-md);
  font-weight: var(--ss-font-medium);
  font-size: var(--ss-text-body);
  transition: background-color var(--ss-transition-fast), color var(--ss-transition-fast);
  cursor: pointer;
}
.ss-btn:focus-visible {
  outline: 2px solid var(--ss-accent-primary);
  outline-offset: 2px;
}

.ss-btn-primary   { background: var(--ss-accent-primary); color: var(--ss-text-on-accent); border: none; }
.ss-btn-primary:hover { background: var(--ss-accent-hover); }

.ss-btn-secondary { background: transparent; color: var(--ss-text-secondary); border: 1px solid var(--ss-border-default); }
.ss-btn-secondary:hover { background: var(--ss-bg-raised); }

.ss-btn-danger    { background: var(--ss-danger); color: var(--ss-text-on-accent); border: none; }

.ss-btn[disabled] { opacity: .55; cursor: not-allowed; }
```

**Rules**

- Destructive actions use danger variant **and** an action verb in the label: `Delete tenant`, `Remove member`, not `Yes` / `Submit`.
- Destructive actions that can't be undone use a **type-to-confirm modal** (existing pattern — see Delete Tenant, Delete User).
- Button labels are **sentence case**. ("Sign in", not "Sign In".)
- First word is a verb. `Cancel invitation`, not `Invitation cancel`.

### 5.2 Status badges

```css
.ss-badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: var(--ss-radius-full);
  font-size: var(--ss-text-caption);
  font-weight: var(--ss-font-medium);
  line-height: 1.4;
}

.ss-badge-success { background: var(--ss-success-bg); color: var(--ss-success); }
.ss-badge-danger  { background: var(--ss-danger-bg);  color: var(--ss-danger); }
.ss-badge-warning { background: var(--ss-warning-bg); color: var(--ss-warning); }
.ss-badge-info    { background: var(--ss-info-bg);    color: var(--ss-info); }
.ss-badge-neutral { background: var(--ss-bg-raised);  color: var(--ss-text-muted); }
```

Label text inside a badge, not a raw icon — never use color alone.

### 5.3 Cards

```css
.ss-card {
  background: var(--ss-bg-surface);
  border: 1px solid var(--ss-border-subtle);
  border-radius: var(--ss-radius-lg);
  padding: var(--ss-space-xl);
}
```

Do not nest cards. If a card needs a sub-panel, use a dividing border, not a second card.

### 5.4 Sidebar

```css
.ss-sidebar {
  background: var(--ss-bg-subtle);
  border-right: 1px solid var(--ss-border-subtle);
  width: 220px;
  padding: var(--ss-space-lg);
}

.ss-nav-item {
  display: flex; align-items: center; gap: var(--ss-space-sm);
  padding: 8px 12px;
  border-radius: var(--ss-radius-md);
  color: var(--ss-text-secondary);
  text-decoration: none;
}
.ss-nav-item:hover  { background: var(--ss-bg-raised); color: var(--ss-text-primary); }
.ss-nav-item-active {
  background: color-mix(in srgb, var(--ss-accent-primary) 12%, transparent);
  color: var(--ss-accent-primary);
  border-left: 3px solid var(--ss-accent-primary);
}
```

### 5.5 Tables

```css
.ss-table { width: 100%; border-collapse: collapse; }
.ss-table th {
  text-align: left;
  padding: 10px 12px;
  color: var(--ss-text-muted);
  font-size: var(--ss-text-caption);
  font-weight: var(--ss-font-medium);
  text-transform: uppercase;
  letter-spacing: .04em;
  border-bottom: 1px solid var(--ss-border-default);
}
.ss-table td {
  padding: 12px;
  border-bottom: 1px solid var(--ss-border-subtle);
  font-size: var(--ss-text-body);
}
.ss-table tr:hover { background: var(--ss-bg-raised); }
.ss-table tr.clickable { cursor: pointer; }
```

**Density rule:** SilkStrand is a compliance tool — operators look at long lists. Default to **42px row height** (comfortable, not spacious). Don't pad rows to 60px "for breathing room".

**Action columns** live on the right, right-aligned. Rows can be clickable for navigation; buttons inside rows must call `stopPropagation()` to avoid triggering the row click.

### 5.6 Forms

```css
.ss-field       { display: flex; flex-direction: column; gap: var(--ss-space-xs); }
.ss-label       { font-size: var(--ss-text-body-sm); color: var(--ss-text-secondary); font-weight: var(--ss-font-medium); }
.ss-label .req  { color: var(--ss-danger); margin-left: 2px; }
.ss-input, .ss-select, .ss-textarea {
  background: var(--ss-bg-surface);
  color: var(--ss-text-primary);
  border: 1px solid var(--ss-border-default);
  border-radius: var(--ss-radius-sm);
  padding: 8px 10px;
  font-size: var(--ss-text-body);
}
.ss-input:focus-visible { outline: 2px solid var(--ss-accent-primary); outline-offset: 1px; border-color: var(--ss-accent-primary); }
.ss-help        { font-size: var(--ss-text-caption); color: var(--ss-text-muted); }
.ss-error       { font-size: var(--ss-text-caption); color: var(--ss-danger); }
```

**Rules**

- Labels are **above** inputs, left-aligned, sentence case.
- Required fields are marked with a red asterisk in the label; don't use only placeholder for required-field hints.
- **Validation fires on submit**, not on blur. Surprise-validation (errors appearing as the user types) is infantilizing. Inline errors appear only after submit attempt, then update live until resolved.
- Help text under the field, same line-height as a caption.
- Error messages describe **what** and **how to fix**: "Password must be at least 8 characters." Not: "Invalid password."

### 5.7 Empty states

Use when a list has zero items. A good empty state has three parts:

1. **Icon** (outline, muted)
2. **One-line explanation** of what this surface contains
3. **Primary action** that creates the first item

```text
 [icon]
 No agents registered.
 [ Generate install command ]
```

Minimum markup:

```css
.ss-empty {
  display: flex; flex-direction: column; align-items: center;
  gap: var(--ss-space-md);
  padding: var(--ss-space-3xl) var(--ss-space-xl);
  color: var(--ss-text-muted);
  text-align: center;
}
.ss-empty .icon { color: var(--ss-text-muted); width: 40px; height: 40px; }
```

**Never** show an empty table with empty `<tr>` rows.

### 5.8 Error states (request / page-level)

Three flavors:

- **Inline error** — under a form field, red.
- **Toast / alert banner** — at the top of the page for request failures that aren't field-specific. Dismissible. Danger color background.
- **Page error** — full-page replacement for fatal loads (e.g., "Failed to load tenant"). Offers retry.

```css
.ss-alert-danger {
  background: var(--ss-danger-bg);
  color: var(--ss-danger);
  border: 1px solid color-mix(in srgb, var(--ss-danger) 30%, transparent);
  border-radius: var(--ss-radius-md);
  padding: var(--ss-space-md) var(--ss-space-lg);
}
```

Error messages follow the form-validation rule: **what went wrong + what the user should do**. No raw stack traces. No "Error: undefined".

### 5.9 Modals / dialogs

```css
.ss-modal-backdrop {
  position: fixed; inset: 0;
  background: rgba(0,0,0,.45);
  display: flex; align-items: center; justify-content: center;
  z-index: 100;
}
.ss-modal {
  background: var(--ss-bg-surface);
  border: 1px solid var(--ss-border-subtle);
  border-radius: var(--ss-radius-lg);
  box-shadow: var(--ss-shadow-lg);
  max-width: 520px;
  width: calc(100vw - 40px);
  padding: var(--ss-space-xl);
}
.ss-modal-actions {
  display: flex; justify-content: flex-end; gap: var(--ss-space-sm);
  margin-top: var(--ss-space-xl);
}
```

**Rules**

- Escape key + backdrop click dismiss non-destructive modals. Destructive modals require an explicit Cancel click.
- Destructive confirmations with an **irreversible** action require type-to-confirm (the user types the exact name of the thing being deleted).
- Primary action is on the **right**, destructive action optional on the **left of primary**. Never two green buttons side-by-side.

---

## 6. Iconography

- **Set:** [Lucide](https://lucide.dev) for the React component API. Heroicons as fallback.
- **Style:** outline, 1.5 stroke, monochrome.
- **Size:** 16px inline with body text, 20px for nav items, 40px for empty states.
- **Color:** inherit `color` from parent — never hard-code.

Suggested icons per surface:

| Surface | Icon |
|---|---|
| Dashboard | `layout-dashboard` |
| Agents | `server` |
| Targets | `target` |
| Scans | `radar` |
| Team | `users` |
| Audit Log | `scroll-text` |
| Settings | `settings` |
| Users (backoffice) | `user-cog` |
| Data Centers | `building-2` |
| Tenants | `briefcase` |

---

## 7. Page layout

Standard page structure (top to bottom):

```
  [ sidebar ]  |  [ topbar: product name · right-aligned actions (tenant switcher, user menu) ]
               |
               |  Page title            [ Primary action button ]
               |  Optional subtitle in --ss-text-muted
               |
               |  Card (optional — banner / KPIs)
               |
               |  Card / table / form
```

**Widths**

- Sidebar: 220px fixed.
- Content: `max-width: 1200px`, left-aligned.
- Forms: `max-width: 560px` when they're primary content; full-width inside a table-row modal.

**Responsive breakpoints**

```css
--ss-bp-sm: 640px;   /* compact phone */
--ss-bp-md: 768px;   /* tablet */
--ss-bp-lg: 1024px;  /* laptop */
--ss-bp-xl: 1280px;  /* desktop */
```

Sidebar collapses to a drawer below `md`. Admin-side pages are desktop-first; tenant-side pages should be usable on tablet at minimum.

---

## 8. Voice & tone

### 8.1 Rules

- Direct, technical, calm, minimal.
- Say what the action does. Don't editorialize.
- **Sentence case** everywhere except proper nouns and acronyms.
- Never use "please", "sorry", "oops", or exclamation marks in UI text.
- Use second person ("your") for things scoped to the user; never first person.
- Errors are neutral, not apologetic: "Password must be at least 8 characters." Not "We're sorry, your password is too short."

### 8.2 Examples

| Good | Bad |
|---|---|
| Generate install command | Let's get scanning! |
| Agent connected | Boom! You're online! |
| Scan completed | All done! |
| Rotate key | Key rotation wizard |
| Delete tenant | Remove organization forever?? |
| No agents registered. | 😔 Nothing here yet… |
| Password must be at least 8 characters. | We need a stronger password! |

### 8.3 Date/time formatting

- Absolute dates: `Apr 12, 2026, 8:45 PM` (user locale; use `Intl.DateTimeFormat`).
- Relative times (tables with recent activity): `5m ago`, `2h ago`, `3d ago`. Implemented in `web/src/lib/time.ts`.
- Always show absolute on hover (tooltip) when displaying relative.

### 8.4 Terminology

Keep these consistent:

| Use | Don't use |
|---|---|
| Tenant | Organization, org, customer |
| Agent | Sensor, probe, collector |
| Target | Asset, endpoint |
| Scan | Job, check |
| Bundle | Policy, ruleset |
| Member | User (inside a tenant) |
| Admin (tenant) / Administrator (backoffice) | Owner, manager |

---

## 9. Migration plan

**Current state:** both frontends are ad-hoc light-theme CSS. No token layer. No dark mode.

**Target state:** shared tokens, both themes, system detection, components in this doc.

### Phase 1 — tokens (non-breaking)
Extract `packages/design-tokens/tokens.css`. Both frontends import it. New code uses tokens; existing code untouched.

### Phase 2 — theme switch
Add `data-theme` attribute on `<html>`, theme picker in Settings, pre-mount inline script to prevent flash. Both themes defined in tokens.

### Phase 3 — migrate components
One page per PR. Dashboard → Agents → Targets → Scans → Team → Settings (tenant side); Dashboard → DCs → Tenants → Users → Audit (backoffice). Each PR updates just that page to use the new classes.

### Phase 4 — delete legacy CSS
Once nothing references the old class names, remove from `index.css`.

**Timeline:** ~1-2 days per phase. No hurry; MVP features come first. Track under GitHub milestone `design-system-v1`.

---

## 10. Open questions

Not yet decided. Revisit in v0.2:

- Should the tenant switcher live in the sidebar (like Slack) or the topbar (current)?
- Toast library vs. handwritten notifications?
- Data viz palette — specific defined colors for charts (pass/fail/warning/error in scan results) beyond the status tokens?
- Logo: commission an illustrator or use a typography-only treatment long-term?

---

## Changelog

| Version | Date | Change |
|---|---|---|
| 0.1 | 2026-04-12 | Initial draft. Not yet applied to any shipped UI. |
