# GitHub Copilot Instructions for Card Judge Project

## Project Architecture

This is a Go-based web application using:
- **Backend**: Go with `text/template` templating
- **Frontend**: HTMX for dynamic interactions, minimal JavaScript
- **Database**: MariaDB
- **Static Files**: Embedded using `embed.FS` (requires server restart for changes)

## Core Principles

### 1. Theme System
- **ALL colors must be defined for ALL 8 theme variations** in `src/static/css/colors.css`
- Colors are defined using CSS variables (custom properties)
- Never hardcode colors in HTML or other CSS files
- Each theme has its own color scheme that must be respected
- New UI elements must blend seamlessly with any selected theme

### 2. CSS and Styling
- **Minimize inline styles** - prefer CSS classes
- Define reusable classes in appropriate CSS files:
  - `global.css` - global styles and reusable classes
  - `colors.css` - theme color variables only
  - Page-specific CSS files (e.g., `lobby.css`, `stats.css`) for page-specific styles
- When creating new UI components, create corresponding CSS classes
- Avoid putting `style=""` attributes directly in HTML templates unless absolutely necessary

### 3. HTMX-First Approach
- **Prefer HTMX over JavaScript DOM manipulation**
- Use HTMX attributes for dynamic content updates:
  - `hx-get`, `hx-post`, `hx-put`, `hx-delete` for API calls
  - `hx-target` to specify where content should go
  - `hx-swap` to control how content is swapped
  - `hx-trigger` for event-based updates
  - `hx-swap-oob` for out-of-band swaps (updating multiple parts of the page)
- Use Go templates to generate dynamic values rather than JavaScript
- JavaScript should only be used when HTMX cannot handle the interaction
- Keep JavaScript minimal and focused on client-side interactions that don't involve server state

### 4. Go Templates
- Use Go `text/template` for server-side rendering
- Template files organized in:
  - `src/static/html/pages/` - full page templates
  - `src/static/html/components/` - reusable components
  - `src/static/html/components/table-rows/` - table row templates
  - `src/static/html/components/tables/` - table header templates
- Templates should contain presentation logic
- Use template functions for formatting (e.g., `{{.Date.Format "2006-01-02"}}`)
- Pass data from Go backend, avoid fetching data in JavaScript

### 5. HTML Structure
- Use semantic HTML elements
- Leverage existing CSS classes before creating new ones
- Keep HTML clean and readable
- Avoid deeply nested structures when possible
- Use Bootstrap Icons via classes (e.g., `bi bi-pencil`)

### 5a. Navigation is always a real `<a href>`
**Anything that navigates to another page MUST be wrapped in an anchor.**
Never use `onclick="location.href='...'"`, and never add a JavaScript
click-interceptor for this. Only a real anchor gives ctrl/cmd-click and
middle-click to open a new tab, right-click → "Open in new tab", the hover
URL preview, and correct link semantics for screen readers — a script cannot
reproduce all of that.

❌ **Wrong** (ctrl+click just navigates in place):
```html
<button onclick="window.location.href='/decks'">Card Decks</button>
```

✅ **Correct** (wrap the visual element in an anchor):
```html
<a href="/decks"><button>Card Decks</button></a>
```

✅ **Correct** (top-bar menu row; `no-style` strips the anchor's own
underline/color so the row looks unchanged):
```html
<a href="/account" class="no-style">
    <div class="top-bar-menu-link">Account <i class="bi bi-gear"></i></div>
</a>
```

This does not apply to buttons that perform an action rather than navigate
(`hx-post`, `hx-put`, `hx-delete`, opening a dialog) — those stay plain
buttons. External links additionally take `target="_blank"`.

### 6. State Management
- **Server-side state is the source of truth**
- Use hidden form inputs to maintain state when necessary
- Use HTMX to sync state between client and server
- Avoid maintaining parallel state in JavaScript
- Let the server generate the current state in templates

### 7. Forms and User Input
- Use HTMX on forms for submission without page reload
- Use `hx-target` to show success/error messages
- Form validation should be server-side first
- Client-side validation (HTML5) is acceptable for UX

### 8. File Organization
- **CSS**: `src/static/css/`
- **HTML Templates**: `src/static/html/`
- **JavaScript**: `src/static/js/` (use sparingly)
- **Images**: `src/static/images/`
- **SQL**: `src/static/sql/`
- Keep files organized by feature/page

## Common Patterns

### Dynamic Content Updates
❌ **Wrong** (JavaScript DOM manipulation):
```javascript
document.getElementById('currentPage').textContent = page;
```

✅ **Correct** (HTMX + Go Template):
```html
<span id="page-info" hx-swap-oob="true">Page {{.CurrentPage}}</span>
```

### Styling
❌ **Wrong** (inline styles with hardcoded colors):
```html
<div style="background-color: #dc3545; color: white; padding: 10px;">
```

✅ **Correct** (CSS classes with theme variables):
```html
<div class="danger-zone">
```
```css
.danger-zone {
    background-color: var(--danger-bg);
    color: var(--danger-text);
    padding: 10px;
}
```

### Dynamic Values
❌ **Wrong** (JavaScript to set values):
```javascript
const pageSize = getPageSize();
document.getElementById('pageSize').value = pageSize;
```

✅ **Correct** (Go template):
```html
<input type="hidden" name="pageSize" value="{{.PageSize}}" />
```

## When Adding New Features

1. **Check existing patterns** - look at similar features in the codebase
2. **Define CSS classes** - create reusable classes with theme support
3. **Use HTMX** - prefer HTMX over JavaScript for server interactions
4. **Wrap navigation in `<a href>`** - every new link/button that changes page
   (see 5a); ctrl+click must open a new tab
5. **Test all themes** - verify the feature works with all 8 theme variations
6. **Keep it simple** - follow the existing code organization and style

## Theme Variables Reference

Colors are defined in `src/static/css/colors.css` with variables like:
- `--background-color`
- `--text-color`
- `--primary-color`
- `--secondary-color`
- `--accent-color`
- `--border-color`
- etc.

Always use these variables, never hardcode colors.
