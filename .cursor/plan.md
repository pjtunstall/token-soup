# Refactor +page.svelte — Decomposition Plan

## Goal
Decompose the monolithic `+page.svelte` (580 lines of script + 200 lines of template) into
focused, manageable components. `RolesPanel.svelte` is a relic (not imported anywhere) and
will be rewritten as the canonical extracted component.

---

## Current state

| File | Status |
|---|---|
| `+page.svelte` | 809 lines — too much state + template inlined |
| `AppHeader.svelte` | ✅ already extracted |
| `ChatPanel.svelte` | ✅ already extracted |
| `ChatsGallery.svelte` | ✅ already extracted |
| `ComposeBar.svelte` | ✅ already extracted |
| `ChatBubble.svelte` | ✅ already extracted |
| `AuthForm.svelte` | ✅ already extracted |
| `RolesPanel.svelte` | ❌ orphan — rewrite with new interface |

---

## What remains inlined in +page.svelte

### 1. The `<aside>` roles panel (lines 640–775)
~135 lines of template covering:
- User display-name header button
- Role editor (new/edit textarea + name input + Exit/Save buttons)
- Role list (New role button + per-role select/edit/delete row)
- All the role-editor state (`roleEditorOpen`, `roleEditorIsNew`, `roleEditorRoleId`,
  `roleEditorName`, `roleEditorDescription`, `roleEditorSaving`, `roleEditorError`)
- Associated functions: `closeRoleEditor`, `openNewRoleEditor`, `openExistingRoleEditor`,
  `saveRoleEditor`

### 2. The `persistWarning` banner (lines 596–603)
Small inline `<div>` — 8 lines; worth extracting as a simple `PersistWarning.svelte` snippet
only if it grows, but the panel extraction is the real priority.

### 3. Styles block (lines 783–808)
`menu-action-btn` and `role-option-btn` CSS lives in `+page.svelte`. These belong in
`RolesPanel.svelte` once the panel is extracted.

---

## Proposed decomposition

### Step 1 — Rewrite `RolesPanel.svelte`

Replace the orphaned file with a component that owns all roles-panel state and logic
currently inlined in `+page.svelte`.

**Props (inputs):**
```ts
{
  roleRows: RoleRow[];
  rolesLoading: boolean;
  rolesLoadError: string;
  selectedRoleId: number | null;
  deletingRoleId: number | null;
  displayName: string;           // replaces getDisplayName(session) call
  accessToken: string;           // needed by saveRoleEditor
  onSelectRole: (r: RoleRow) => void;
  onRolesChanged: () => Promise<void>;   // triggers loadRoleRows() in parent
  onDeleteRole: (r: RoleRow, e: MouseEvent) => void;
  onClose: () => void;           // called when editor is closed (parent clears closeRoleEditor())
}
```

**Internal state (moves out of +page.svelte):**
- `roleEditorOpen`, `roleEditorIsNew`, `roleEditorRoleId`
- `roleEditorName`, `roleEditorDescription`
- `roleEditorSaving`, `roleEditorError`

**Exported action (bindable / `export function`):**
- `closeEditor()` — parent calls this on `sendMessage`, `stopMessage`, `openChatFromList`,
  `handleNewChat`, `handleChatGallery`, `handleSpeechBubble`

**Template:** the full `<aside>` block, including the user-name header button and the
`menu-action-btn` / `role-option-btn` styles.

**Also moves:** `roleNameExists` duplicate-check logic from `saveRoleEditor` — stays inside
the component.

---

### Step 2 — Slim down +page.svelte

After extracting `RolesPanel`:

**State removed from +page.svelte:**
- `roleEditorOpen`, `roleEditorIsNew`, `roleEditorRoleId`, `roleEditorName`,
  `roleEditorDescription`, `roleEditorSaving`, `roleEditorError`

**Functions removed:**
- `closeRoleEditor`, `openNewRoleEditor`, `openExistingRoleEditor`, `saveRoleEditor`

**Functions simplified** (no longer call `closeRoleEditor()` directly — call panel ref instead):
- `sendMessage`, `stopMessage`, `openChatFromList`, `handleNewChat`,
  `handleChatGallery`, `handleSpeechBubble`

**Remaining state that stays in +page.svelte** (orchestration layer):
- Auth: `authState`, `session`
- Chat: `messages`, `messageInput`, `awaitingAi`, `activeAiJobId`, `logContainer`,
  `aiBubbleIndexByJobId`, `reasoningExpandedByIndex`, `scrollRaf`, `abortController`
- Session: `sessionSubtitle`, `chatId`, `persistWarning`, `persistEphemeral`, `signedInMain`
- Roles (data only): `roleRows`, `rolesLoadError`, `rolesLoading`, `deletingRoleId`,
  `selectedRoleId`
- Chat gallery: `chatRows`, `chatsLoadError`, `chatsLoading`

**Template after extraction:**
```svelte
<aside ...>
  <RolesPanel
    bind:this={rolesPanelRef}
    {roleRows} {rolesLoading} {rolesLoadError}
    {selectedRoleId} {deletingRoleId}
    displayName={getDisplayName(session)}
    accessToken={session?.access_token ?? ''}
    onSelectRole={selectRoleAsCurrent}
    onRolesChanged={loadRoleRows}
    onDeleteRole={deleteRoleRow}
  />
</aside>
```

---

## Files changed

| File | Action |
|---|---|
| `RolesPanel.svelte` | **Rewrite** (orphan → real component) |
| `+page.svelte` | Remove inlined aside/editor state/logic; add `<RolesPanel>` |

No other files need to change.

---

## What we are NOT doing (keeping simple)
- Not extracting `persistWarning` (too small to justify a file)
- Not creating a state store/context (Svelte 5 runes + prop-drilling is working fine)
- Not moving `getDisplayName`, `selectedRoleStorageKey`, `syncSelectedRoleAfterLoad`,
  `selectRoleAsCurrent` into the panel (they depend on `session` which lives in the page)
- Not touching `AppHeader`, `ChatPanel`, `ChatsGallery`, etc.
