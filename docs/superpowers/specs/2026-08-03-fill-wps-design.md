# Fill Wps Design

**Goal:** Define how `fillWpsNode` fills one work permit template from `state`, structured memory, and leaves unresolved fields blank.

## Scope

This design covers:
- Single work permit filling algorithm
- Node inputs and outputs
- Structured memory data model

This design does not cover:
- RAG retrieval
- Memory persistence or indexing implementation
- Submission or validation after filling

## Inputs

`fillWpsNode` reads:
- `state.WpTemplates`
- `state.Area`
- `state.StartTime`
- `state.EndTime`
- `state.Description`
- `state.Equipments`
- `state.WpCompany`

It also receives structured memory entries keyed by:
- work permit type
- work area

## Structured Memory

Structured memory stores historical fill results from past work permits.

Recommended data shape:

```go
type StructuredMemoryEntry struct {
    WpTypeID   int
    AreaID     int
    FieldName  string
    FieldValue any
    Source     string
    UpdatedAt  int64
}
```

Memory is queried by:
- exact work permit type match
- exact area match

If multiple entries match the same field, prefer the newest entry.

## Filling Order

Field filling uses this order:
1. `state`
2. structured memory
3. leave blank

The node must not invent values.

## Output

Each filled work permit should record:
- filled fields
- unfilled fields
- field source list

Recommended output shape:

```go
type FilledField struct {
    Name   string
    Value  any
    Source string
}

type UnfilledField struct {
    Name string
}
```

`state.FilledWps` should hold one result per template.

## Behavior

For each template:
- inspect the template fields to determine required values
- try to fill each field from `state`
- if missing, check structured memory
- if still missing, leave empty
- collect source metadata for every filled field
- collect missing field names for every unfilled field

## Error Handling

Return an error when:
- a template is malformed
- structured memory lookup fails
- a field cannot be mapped to any recognized source path

Do not return an error for ordinary missing data.
Missing data should be reported through the unfilled field list.

## Testing

Add tests for:
- state-first precedence
- structured-memory fallback
- blank output for unresolved fields
- field source reporting
- multiple templates in one run

