# Architecture & Best Practices

`vtui` is designed to build heavy-duty, stateful Terminal User Interfaces in Go. To achieve high performance and maintainable code, the framework relies on specific architectural patterns.

This document explains the "Why" behind the framework's design and outlines best practices for contributing to or using `vtui`.

---

## 1. The UI Object Model: Embedding vs. Composition

If you look at the source code for widgets like `ListBox`, `Table`, or `TreeView`, you will notice they embed a heavy `ScrollView` struct, which in turn embeds `ScreenObject`.

**Why `ScrollView` instead of decoupled components (e.g., `Scrollable`, `Focusable`, `Selectable`)?**

In languages with classic OOP (like C++ or Java), complex UI widgets are often built using multiple inheritance or deep composition. Go lacks inheritance. If we used strict composition (where a `ListBox` *contains* a `Scroller` field rather than *embedding* it), we would encounter two major issues:

1. **Massive Boilerplate:** Every widget would need dozens of proxy methods. To scroll a list, you'd have to write `func (l *ListBox) ScrollUp() { l.scroller.ScrollUp() }` for every single action.
2. **Cyclic Dependencies:** A `Scroller` needs to know the screen height, which is known by the `ListBox`, creating tight coupling.

**The Solution:** Go's struct embedding (`type ListBox struct { ScrollView }`) provides a clean way to inherit behavior. `ScrollView` handles all the complex math for `TopPos`, `SelectPos`, PageUp/PageDown, and scrollbar mouse clicks. The widget only needs to implement the rendering (`DisplayObject`) and data mapping. We achieve polymorphism via lightweight interfaces (like `SelectableRow` or `CommandHandler`).

---

## 2. Event Handling: Callbacks vs. Command Routing

`vtui` supports two different ways to handle user actions:

1. **Callbacks (Go-style):** `btn.OnClick = func() { doSomething() }`
2. **Command Routing (Turbo Vision-style):** `btn.Command = vtui.CmCopy`

**Why keep both?**

We evaluated building a strict, unified Event Bus for everything, or entirely removing callbacks. However, we chose this hybrid approach for a very specific reason: **Developer Velocity and Migration.**

* **Callbacks** are incredible for fast prototyping. They provide Go's native type safety and let you build a simple dialog in one function without declaring global command IDs.
* However, relying *only* on callbacks in a massive application (like `f4`) leads to "callback hell" (spaghetti code) and closures that trap memory.
* **Commands** allow for clean MVC (Model-View-Controller) decoupling. A button emits `CmSave`, and the parent Window, or the global Application, catches it via `HandleCommand`.

**The `FireAction` priority:**
Widgets use the internal `FireAction` method. If a user sets `OnClick`, it executes. If not, it looks for `Command` and bubbles it up the ownership tree. This allows a developer to "slap together" an interface using callbacks, and later seamlessly refactor it to Commands without creating technical debt.

**Rule of Thumb:**
* Use **Callbacks** for local UI state changes (e.g., a checkbox that shows/hides a password field).
* Use **Commands** for business logic and cross-component communication (e.g., closing windows, saving files, changing views).

---

## 3. Rendering & Performance (Zero-Allocation)

Garbage Collection (GC) pauses are fatal for TUI responsiveness, especially when users type rapidly or paste large blocks of text. `vtui` is strictly designed to minimize allocations during the hot render loop.

### The `ScreenBuf` and `Flush()`
* `vtui` uses double-buffering. You draw to an in-memory array of `CharInfo` cells.
* `Flush()` compares this array to a shadow buffer. It generates ANSI sequences **only for the cells that changed**.
* The ANSI sequences are written to a `strings.Builder` and sent to `os.Stdout` in a single `Write()` call.

### Best Practices for Rendering
1. **Never allocate memory in `DisplayObject` or `Show`:** Do not use `fmt.Sprintf` to format strings inside the drawing loop if it can be avoided. Pre-format strings or use `strconv` / custom formatters.
2. **Reuse Slices:** If your widget needs to process slices of cells (like `EditorView`), allocate the slices once in the struct (`renderCells []vtui.CharInfo`) and `target = target[:0]` before reusing them.
3. **Avoid `time.Sleep` in UI Thread:** The UI thread must never block. Heavy operations (reading files, searching) MUST be wrapped in `vtui.RunAsync(...)`.

---

## 4. Focus and Navigation Guidelines

### Committed text and shortcuts

`vtinput.InputEvent.TextInput` identifies committed modified or composed text.
Its physical key, scan code, modifiers and `InputSource` remain available for
recording and terminal forwarding. Applications must check this flag before
remapping keys or matching commands and macro triggers; event filters may still
observe and record the original event. Ordinary unmodified typing keeps its
existing character-shortcut behavior.

Text controls use `TextInputView` to obtain an insertion-only copy without
shortcut modifiers or a virtual key. Dialog and menu accelerators, workspace
shortcuts and default-button fallbacks must not consume a marked event. Text
filtering remains available. A marked event's `RepeatCount` applies to its
committed rune; recording and forwarding must retain the original event rather
than the insertion view.

Native Windows GUI translation is inspected on the event thread before
`TranslateMessage`, with the non-mutating `ToUnicodeEx` flag (Windows 10 build
14393 or newer). A text-producing press waits for the native character message;
a dead key has no timed shortcut fallback. UTF-16 surrogate pairs form one rune.
Terminal protocol events must never be translated through the local GUI layout.
The Windows gogpu host uses `App.OnNativeKey` for the same source-time decision
and metadata. It registers the legacy keyboard callbacks only when that native
stream is unavailable, preventing duplicate delivery. It never probes a Windows
layout when a queued gogpu callback reaches the application.

To maintain a predictable "Desktop TUI" feel:

* **Tab / Shift+Tab:** Must ALWAYS predictably cycle through all focusable elements in a group. Do not trap `Tab` inside an element unless explicitly required (like a multiline text editor).
* **Arrow Keys:** Used for *internal* navigation (moving up/down a list). When the cursor hits the boundary of a component (e.g., pressing Up on the first item of a list), the widget should return `false` from `ProcessKey`, allowing the parent group to pass focus to the previous/next element.
