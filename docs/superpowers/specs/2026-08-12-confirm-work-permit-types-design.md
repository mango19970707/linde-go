# Work Permit Type Confirmation Design

## Goal

After intent recognition identifies the required work permit types, optionally pause the graph and ask the user to confirm or edit the list before template lookup and permit generation. Confirmation is enabled by default.

## Design

- Add `work_permit_api.confirm_wp_types` as an optional boolean configuration.
- Treat an omitted value as `true`; an explicit `false` skips confirmation.
- Add a `confirmWpTypes` graph step inside `intentRecognition`:
  - On the first run, recognize types and interrupt with `operation: confirm_wp_types`.
  - Return the recognized `intent_wp_types` and a confirmation question in the interrupt payload.
  - On resume, accept a typed confirmation payload and replace the recognized list with the user-submitted list.
- Re-query the site's work permit metadata when applying edited types and validate all submitted template/type IDs against the current metadata.
- Continue to `searchWpTemplate` after confirmation or when confirmation is disabled.
- Extend the HTTP resume contract with `confirmed` and `intent_wp_types`, and expose `intent_wp_types` in interrupted responses.

## Error Handling

- Missing confirmation data is rejected with a validation error.
- A modified list containing unknown template or work permit type IDs is rejected.
- Existing information collection and submission approval flows remain unchanged.

## Testing

- Unit test intent recognition interruption and resume with edited types.
- Unit test confirmation-disabled flow.
- HTTP test confirmation preview and resume payload mapping.
- Config test omitted/default and explicit false values.
